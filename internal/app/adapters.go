package app

import (
	"context"
	"sort"
	"time"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/internal/adapter"
)

type RouteReport struct {
	Name         string            `json:"name"`
	Available    bool              `json:"available"`
	Installed    bool              `json:"installed"`
	Trusted      bool              `json:"trusted"`
	Trust        adapter.TrustKind `json:"trust"`
	StableKey    bool              `json:"stable_key"`
	TrustSummary string            `json:"trust_summary"`
	Detail       string            `json:"detail"`
	LastEvent    *time.Time        `json:"last_event,omitempty"`
	Error        string            `json:"error,omitempty"`
}

type CapabilityReport struct {
	Point     string `json:"point"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

type FieldReport struct {
	Point       string `json:"point"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	Source      string `json:"source,omitempty"`
	Approximate bool   `json:"approximate"`
	Reason      string `json:"reason,omitempty"`
}

type StaticReport struct {
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
}

type AdapterReport struct {
	Provider          string             `json:"provider"`
	Detected          bool               `json:"detected"`
	Version           string             `json:"version"`
	ConfigDir         string             `json:"config_dir"`
	DetectedConfigDir string             `json:"detected_config_dir"`
	Routes            []RouteReport      `json:"routes"`
	Capabilities      []CapabilityReport `json:"capabilities"`
	Fields            []FieldReport      `json:"fields"`
	Static            []StaticReport     `json:"static"`
	Limit             NativeLimit        `json:"limit"`
	Error             string             `json:"error,omitempty"`
}

type AdaptersResult struct {
	Adapters []AdapterReport `json:"adapters"`
}

// Adapters inventories every registered adapter and every route. Detection or
// route failures are report data, so one broken provider cannot hide another.
func (s *Service) Adapters(ctx context.Context) (AdaptersResult, error) {
	return s.inspectAdapters(ctx, lockDocument{})
}

func (s *Service) inspectAdapters(ctx context.Context, lock lockDocument) (AdaptersResult, error) {
	if err := checkContext(ctx); err != nil {
		return AdaptersResult{}, err
	}
	providers := make([]adapter.ProviderID, 0, len(s.definitions))
	for provider := range s.definitions {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providerOrder(providers[i]) < providerOrder(providers[j]) ||
			providerOrder(providers[i]) == providerOrder(providers[j]) && providers[i] < providers[j]
	})
	result := AdaptersResult{Adapters: []AdapterReport{}}
	for _, provider := range providers {
		if err := ctx.Err(); err != nil {
			return AdaptersResult{}, err
		}
		definition := s.definitions[provider]
		installation, detected, err := definition.Detect(ctx)
		report := AdapterReport{Provider: string(provider), Detected: detected,
			Routes: []RouteReport{}, Capabilities: []CapabilityReport{}, Fields: []FieldReport{}, Static: []StaticReport{}}
		if err != nil {
			report.Error = err.Error()
		}
		report.DetectedConfigDir = installation.ConfigDir
		// Inspect the root actually installed, even when the invoking process
		// now detects a different configuration directory or no binary.
		if locked, ok := lock.Harnesses[string(provider)]; ok {
			installation.Provider, installation.ConfigDir = provider, locked.ConfigDir
			if installation.Version == "" {
				installation.Version = locked.Version
			}
		}
		report.Version, report.ConfigDir = installation.Version, installation.ConfigDir
		for _, route := range definition.Routes {
			report.Routes = append(report.Routes, inspectRoute(ctx, route, installation, detected))
		}
		fillAdapterFacts(&report, definition, installation)
		result.Adapters = append(result.Adapters, report)
	}
	return result, nil
}

func inspectRoute(ctx context.Context, route adapter.Route, installation adapter.Installation, detected bool) RouteReport {
	if binder, ok := route.(adapter.InstallationBinder); ok {
		route = binder.BindInstallation(installation)
	}
	trust := route.Trust()
	report := RouteReport{Name: route.Name(), Trust: trust.Kind, StableKey: trust.StableKey, TrustSummary: trust.Summary}
	available, reason, availableErr := route.Available(ctx, installation)
	status, statusErr := route.Status(ctx, installation)
	report.Available = detected && available && status.Available
	report.Installed, report.Trusted, report.Detail = status.Installed, status.Trusted, status.Detail
	if report.Detail == "" {
		report.Detail = reason
	}
	if !status.LastEvent.IsZero() {
		report.LastEvent = &status.LastEvent
	}
	if availableErr != nil {
		report.Error = availableErr.Error()
	}
	if statusErr != nil {
		if report.Error != "" {
			report.Error += "; "
		}
		report.Error += statusErr.Error()
	}
	return report
}

func fillAdapterFacts(report *AdapterReport, definition adapter.Definition, installation adapter.Installation) {
	points := []string{plan.SessionStart, plan.PromptSubmitted, plan.SkillExplicit, plan.ToolPre, plan.ToolPost, plan.ToolError, plan.SubagentStart}
	fields := []string{"model", "thinking_effort", "thinking_effort_raw", "thinking_effort_approx", "harness.cwd", "harness.turn_id", "harness.agent", "tool.name", "tool.input", "tool.output", "tool.added", "mcp.server", "mcp.tool", "skill.name"}
	for _, point := range points {
		for _, capability := range []plan.Capability{plan.AddContext, plan.SystemMessage, plan.Deny} {
			value := definition.Facts.Capability(point, capability)
			report.Capabilities = append(report.Capabilities, CapabilityReport{point, string(capability), value.Available, value.Reason})
		}
		for _, field := range fields {
			value := definition.Facts.DataSource(point, field)
			report.Fields = append(report.Fields, FieldReport{point, field, value.Available, value.Source, value.Approximate, value.Reason})
		}
	}
	for _, slot := range definition.StaticPositions.StaticPositions(installation) {
		report.Static = append(report.Static, StaticReport{slot.Name, slot.Mode, slot.Supported, slot.Reason})
	}
	limit := definition.Limits
	report.Limit = NativeLimit{Value: limit.Input.Value, Unit: string(limit.Input.Unit), Approximate: limit.Approximate, Source: limit.Source, Overflow: string(limit.Overflow)}
}
