package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
)

// PlanRequest selects one or more provider plan reports. "auto" selects
// providers detected on the current host.
type PlanRequest struct {
	Source          string
	Ref, Path, Home string
	Harnesses       []string
	AssumeRequests  int
	AssumeTriggers  int
}

// PlanReport combines the core placement report and token report for one
// provider. Bodies and template values are deliberately absent.
type PlanReport struct {
	Provider    string        `json:"provider"`
	Route       string        `json:"route"`
	NativeLimit NativeLimit   `json:"native_limit"`
	Budget      BudgetSummary `json:"budget"`
	Plan        plan.Report   `json:"plan"`
	Tokens      tokens.Report `json:"tokens"`
	SessionCost SessionCost   `json:"session_cost"`
	Diagnostics []Diagnostic  `json:"diagnostics,omitempty"`
}

// NativeLimit describes the provider input ceiling used by runtime hooks.
// It is separate from STYLE package budgets because the units and ownership
// differ.
type NativeLimit struct {
	Value       int    `json:"value"`
	Unit        string `json:"unit"`
	Approximate bool   `json:"approximate"`
	Source      string `json:"source,omitempty"`
	Overflow    string `json:"overflow"`
}

// BudgetSummary is the package-level token budget used by the analyzer.
type BudgetSummary struct {
	PerRule  float64 `json:"per_rule"`
	PerEvent float64 `json:"per_event"`
	AlwaysOn float64 `json:"always_on"`
}

// SessionCost compares rule estimates under a hypothetical 1.25x write / 0.1x
// later-hit cache scenario. It is not provider usage or a billing estimate.
type SessionCost struct {
	Requests  int     `json:"requests"`
	Triggers  int     `json:"triggers"`
	AlwaysOn  float64 `json:"always_on"`
	Triggered float64 `json:"triggered"`
	Total     float64 `json:"total"`
}

// PlanResult is the deterministic output of a plan request.
type PlanResult struct {
	Source      *SourceReport `json:"source,omitempty"`
	Package     string        `json:"package"`
	Version     string        `json:"version"`
	Diagnostics []Diagnostic  `json:"diagnostics,omitempty"`
	Reports     []PlanReport  `json:"reports"`
}

// UsageError identifies a plan request that cannot be interpreted without
// changing the package or consulting a provider.
type UsageError struct{ Message string }

func (err *UsageError) Error() string { return err.Message }

// Plan loads, validates, merges, plans, and estimates a package without
// changing any files.
func (s *Service) Plan(ctx context.Context, request PlanRequest) (PlanResult, error) {
	if err := checkContext(ctx); err != nil {
		return PlanResult{}, err
	}
	validation, err := s.Validate(ctx, ValidateRequest{Source: request.Source, Ref: request.Ref, Path: request.Path, Home: request.Home})
	if err != nil {
		return PlanResult{}, err
	}
	result := PlanResult{Source: validation.Source, Diagnostics: packageDiagnostics(validation.Diagnostics)}
	if validation.Package == nil {
		return result, nil
	}
	metadata := validation.Package.Metadata
	result.Package, _ = metadata["name"].(string)
	result.Version, _ = metadata["version"].(string)
	if hasErrors(result.Diagnostics) {
		return result, nil
	}
	resolved, err := spec.Merge(validation.Package)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, parseDiagnostic(err))
		return result, nil
	}
	providers, err := s.selectProviders(ctx, request.Harnesses)
	if err != nil {
		return result, err
	}
	for _, provider := range providers {
		definition := s.definitions[provider]
		installation, detected, err := definition.Detect(ctx)
		if err != nil {
			return result, err
		}
		// An explicitly requested, undetected provider still supports offline
		// planning. Installation must establish its version before writing.
		if detected {
			if finding := harnessRequirement(metadata, provider, installation.Version); finding != nil {
				result.Diagnostics = append(result.Diagnostics, *finding)
				continue
			}
		}
		planner, err := adapter.BuildPlanAdapter(provider, definition.Facts)
		if err != nil {
			return result, err
		}
		planned, err := plan.Build(resolved, planner)
		if err != nil {
			return result, fmt.Errorf("plan %s: %w", provider, err)
		}
		budget, err := tokens.AnalyzeWithLayout(resolved, planned, layoutFor(provider), validation.source...)
		if err != nil {
			return result, fmt.Errorf("token plan %s: %w", provider, err)
		}
		cost := estimateSessionCost(resolved, planned, budget, request.AssumeRequests, request.AssumeTriggers)
		result.Reports = append(result.Reports, PlanReport{
			Provider: string(provider), Route: preferredRoute(definition),
			NativeLimit: NativeLimit{
				Value: definition.Limits.Input.Value, Unit: string(definition.Limits.Input.Unit),
				Approximate: definition.Limits.Approximate, Source: definition.Limits.Source,
				Overflow: string(definition.Limits.Overflow),
			},
			Budget: budgetSummary(resolved), Plan: planned, Tokens: budget, SessionCost: cost,
		})
	}
	return result, nil
}

func budgetSummary(pkg *spec.ResolvedPackage) BudgetSummary {
	limits := tokens.PackageLimits(pkg)
	return BudgetSummary{PerRule: limits.PerRule, PerEvent: limits.PerEvent, AlwaysOn: limits.AlwaysOn}
}

func preferredRoute(definition adapter.Definition) string {
	if len(definition.Routes) == 0 || definition.Routes[0] == nil {
		return "none"
	}
	return definition.Routes[0].Name()
}

func packageDiagnostics(values []Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(values))
	for _, value := range values {
		if value.Provider != "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func estimateSessionCost(pkg *spec.ResolvedPackage, planned plan.Report, _ tokens.Report, requests, triggers int) SessionCost {
	if requests <= 0 {
		requests = 60
	}
	if triggers <= 0 {
		triggers = 3
	}
	if triggers > requests {
		triggers = requests
	}
	cost := SessionCost{Requests: requests, Triggers: triggers}
	for index, rule := range pkg.Rules {
		if index >= len(planned.Rules) {
			continue
		}
		if planned.Rules[index].DeduplicatedInto != "" {
			continue
		}
		requirement, err := plan.RequirementFor(rule)
		if err != nil {
			continue
		}
		for _, placement := range planned.Rules[index].Placements {
			estimate, err := tokens.EstimatePlacement(rule, placement, pkg)
			if err != nil || estimate.Tokens == 0 {
				continue
			}
			if placement.StaticSlot != "" || placement.Event == plan.SessionStart && !requirement.HasWhen {
				cost.AlwaysOn += float64(estimate.Tokens) * cacheCostFactor(1, requests)
				continue
			}
			count := triggers
			if requirement.Repeat == "session" {
				count = 1
			}
			for trigger := 1; trigger <= count; trigger++ {
				position := int(math.Round(float64(trigger*requests) / float64(count+1)))
				if position < 1 {
					position = 1
				}
				cost.Triggered += float64(estimate.Tokens) * cacheCostFactor(position, requests)
			}
		}
	}
	cost.Total = cost.AlwaysOn + cost.Triggered
	return cost
}

func cacheCostFactor(position, requests int) float64 {
	return 1.25 + 0.1*float64(requests-position)
}

func (s *Service) selectProviders(ctx context.Context, values []string) ([]adapter.ProviderID, error) {
	if len(values) == 0 {
		values = []string{"auto"}
	}
	selected := make(map[adapter.ProviderID]bool)
	requested := make(map[adapter.ProviderID]bool)
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(strings.ToLower(value))
			if value == "" {
				continue
			}
			if value == "auto" {
				for _, provider := range s.providers() {
					definition := s.definitions[provider]
					_, installed, err := definition.Detect(ctx)
					if err != nil {
						return nil, err
					}
					if installed {
						selected[provider] = true
					}
				}
				continue
			}
			if value == "claude" {
				value = "claude-code"
			}
			provider := adapter.ProviderID(value)
			if _, ok := s.definitions[provider]; !ok {
				return nil, &UsageError{Message: fmt.Sprintf("unknown harness %q", value)}
			}
			requested[provider] = true
		}
	}
	for provider := range requested {
		selected[provider] = true
	}
	providers := make([]adapter.ProviderID, 0, len(selected))
	for provider := range selected {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		order := func(value adapter.ProviderID) int {
			if value == "codex" {
				return 0
			}
			if value == "claude-code" {
				return 1
			}
			return 2
		}
		ei, ej := order(providers[i]), order(providers[j])
		if ei != ej {
			return ei < ej
		}
		return providers[i] < providers[j]
	})
	if len(providers) == 0 {
		return nil, &UsageError{Message: "no harness selected"}
	}
	return providers, nil
}

type providerLayout string

func layoutFor(provider adapter.ProviderID) tokens.Layout { return providerLayout(provider) }

func (layout providerLayout) Buckets(rule spec.Rule, placement plan.Placement) []tokens.Bucket {
	if placement.StaticSlot != "" {
		return []tokens.Bucket{{Key: "static:" + placement.StaticSlot, Label: "static " + placement.StaticSlot}}
	}
	event := placement.Event
	label := event
	switch event {
	case plan.SessionStart:
		label = "SessionStart"
	case plan.PromptSubmitted:
		label = "UserPromptSubmit"
	case plan.SkillExplicit:
		label = "UserPromptSubmit"
		if layout == "claude-code" {
			label = "UserPromptExpansion"
		}
	case plan.ToolPre:
		label = "PreToolUse"
	case plan.ToolPost:
		label = "PostToolUse"
	case plan.ToolError:
		label = "PostToolUseFailure"
	case plan.SubagentStart:
		label = "SubagentStart"
	}
	if event == plan.SkillExplicit && layout == "claude-code" && rule.Effective["target"] == "skill" {
		groups := layout.selectorGroups(rule, event)
		result := make([]tokens.Bucket, 0, len(groups))
		for _, group := range groups {
			bucketLabel := label + " " + group
			result = append(result, tokens.Bucket{Key: string(layout) + "\x00" + bucketLabel, Label: bucketLabel})
		}
		return result
	}
	if event == plan.ToolPre || event == plan.ToolPost || event == plan.ToolError {
		groups := layout.selectorGroups(rule, event)
		if len(groups) == 0 {
			groups = []string{"*"}
		}
		result := make([]tokens.Bucket, 0, len(groups))
		for _, group := range groups {
			bucketLabel := label + " " + group
			result = append(result, tokens.Bucket{Key: string(layout) + "\x00" + bucketLabel, Label: bucketLabel})
		}
		return result
	}
	return []tokens.Bucket{{Key: string(layout) + "\x00" + label, Label: label}}
}

func (layout providerLayout) selectorGroups(rule spec.Rule, event string) []string {
	values := make([]string, 0)
	appendValues := func(raw any) {
		switch value := raw.(type) {
		case string:
			if value != "" {
				values = append(values, value)
			}
		case []any:
			for _, item := range value {
				if text, ok := item.(string); ok && text != "" {
					values = append(values, text)
				}
			}
		case []string:
			values = append(values, value...)
		}
	}
	if rule.Effective["target"] == "skill" {
		if event != plan.SkillExplicit && layout == "codex" {
			return []string{"Bash"}
		}
		if event == plan.SkillExplicit && layout == "claude-code" {
			appendValues(rule.Effective["skill"])
			if len(values) > 0 {
				sort.Strings(values)
				return uniqueStrings(values)
			}
			return []string{"*"}
		}
		return []string{"Skill"}
	}
	if rule.Effective["target"] == "mcp" {
		// A native MCP hook receives all MCP calls in one bucket. The exact
		// server/tool selector remains a runtime match condition, but it must
		// not split the native input budget into one bucket per server.
		return []string{"mcp"}
	}
	// The runtime accepts either selector family, so include both when both
	// are present; canonical selectors expand to every native matcher they can
	// reach.
	appendValues(rule.Effective["names"])
	appendValues(rule.Effective["tool"])
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		switch value {
		case "shell":
			value = "Bash"
		case "file.edit":
			if layout == "codex" {
				value = "apply_patch"
			} else {
				// Claude exposes several native edit tools for the same
				// canonical file.edit capability. Count each possible native
				// matcher because a rule may be routed to any of them.
				for _, native := range []string{"Edit", "MultiEdit", "NotebookEdit"} {
					if !seen[native] {
						seen[native] = true
						result = append(result, native)
					}
				}
				continue
			}
		case "file.write":
			if layout != "claude-code" {
				continue
			}
			value = "Write"
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func uniqueStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
