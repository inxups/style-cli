package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/spec"
)

type fakeAdapter struct {
	name    string
	caps    map[string]map[Capability]bool
	data    map[string]map[string]bool
	slots   map[string]string
	blindID string
}

func (a fakeAdapter) Name() string { return a.name }

func (a fakeAdapter) Supports(event string, capability Capability) bool {
	return a.caps[event][capability]
}

func (a fakeAdapter) DataSource(event, field string) bool {
	fields, exists := a.data[event]
	if !exists {
		return true
	}
	available, exists := fields[field]
	if !exists {
		return true
	}
	return available
}

func (a fakeAdapter) BlindSpot(rule spec.Rule) (string, bool) {
	if a.blindID != "" && rule.Fields["id"] == a.blindID {
		return "target is outside the adapter subscription", true
	}
	return "", false
}

func (a fakeAdapter) StaticLocation(mode string) (string, bool) {
	slot, ok := a.slots[mode]
	return slot, ok
}

func TestRequirementForDerivesEventsNeedsAndCapabilities(t *testing.T) {
	rule := spec.Rule{
		Source: "rules/review.md",
		Body:   "Review {{model}} before continuing.",
		Effective: map[string]any{
			"id":       "review",
			"target":   "tool",
			"action":   "warn",
			"at":       "before",
			"fallback": "degrade",
			"repeat":   "every",
			"when": map[string]any{
				"model": "gpt-*",
				"conditions": []any{map[string]any{
					"field":   "tool.input.command",
					"op":      "contains",
					"pattern": "git",
				}},
			},
		},
	}
	requirement, err := RequirementFor(rule)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requirement.Events, []string{ToolPre, ToolPost}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
	if got, want := requirement.AnyCapabilities, []Capability{AddContext, SystemMessage}; !reflect.DeepEqual(got, want) {
		t.Fatalf("warn capabilities = %#v, want %#v", got, want)
	}
	if !requirement.HasWhen || requirement.StaticBody {
		t.Fatalf("requirement flags = when:%t static:%t", requirement.HasWhen, requirement.StaticBody)
	}
	if got, want := requirement.Needs, []string{"agent", "model", "tool.input.command"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("needs = %#v, want %#v", got, want)
	}
}

func TestBuildNativeAndDegradedToolPlacements(t *testing.T) {
	native := fakeAdapter{
		name: "codex",
		caps: map[string]map[Capability]bool{ToolPre: {AddContext: true}},
	}
	rule := rule("edit", "tool", "inject", "before", "degrade", "Review the edit.")
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, native)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Rules[0].Status; got != Native {
		t.Fatalf("native status = %q, want %q; report=%s", got, Native, report.String())
	}
	if got := report.Rules[0].Placements[0].Event; got != ToolPre {
		t.Fatalf("native event = %q, want %q", got, ToolPre)
	}

	degraded := fakeAdapter{
		name: "codex",
		caps: map[string]map[Capability]bool{ToolPost: {AddContext: true}},
	}
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, degraded)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Degraded || len(planned.Placements) != 1 {
		t.Fatalf("degraded plan = %#v, want one degraded placement", planned)
	}
	placement := planned.Placements[0]
	if placement.Event != ToolPost || placement.DegradedFrom != ToolPre {
		t.Fatalf("degraded placement = %#v", placement)
	}
}

func TestBuildWarnPartialAndBlockDegrade(t *testing.T) {
	adapter := fakeAdapter{
		name: "codex",
		caps: map[string]map[Capability]bool{
			ToolPre: {AddContext: true},
		},
	}
	warnRule := rule("warn", "tool", "warn", "before", "degrade", "Please check.")
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{warnRule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Rules[0].Status; got != Partial {
		t.Fatalf("partial warn status = %q, want %q; report=%s", got, Partial, report.String())
	}
	if got := report.Rules[0].Placements[0].Capabilities; !reflect.DeepEqual(got, []Capability{AddContext}) {
		t.Fatalf("partial warn capabilities = %#v", got)
	}

	blockRule := rule("block", "tool", "block", "before", "degrade", "Do not continue.")
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{blockRule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	placement := report.Rules[0].Placements[0]
	if placement.Action != "warn" || placement.Status != Partial {
		t.Fatalf("block fallback placement = %#v, want partial warn", placement)
	}
	if !strings.Contains(strings.Join(placement.Reasons, " "), "downgraded to warn") {
		t.Fatalf("block fallback reason = %#v", placement.Reasons)
	}
}

func TestBuildSystemStaticAndDynamicPlacements(t *testing.T) {
	staticAdapter := fakeAdapter{
		name:  "codex",
		slots: map[string]string{"append": "developer_instructions"},
	}
	staticRule := rule("system", "system", "inject", "", "degrade", "Always review changes.")
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{staticRule}}, staticAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Rules[0].Placements[0].StaticSlot; got != "developer_instructions" {
		t.Fatalf("static slot = %q", got)
	}

	dynamicAdapter := fakeAdapter{
		name: "codex",
		caps: map[string]map[Capability]bool{
			SessionStart:    {AddContext: true},
			PromptSubmitted: {AddContext: true},
		},
	}
	dynamicRule := rule("dynamic", "system", "inject", "", "degrade", "Current model: {{model}}")
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{dynamicRule}}, dynamicAdapter)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Native || len(planned.Placements) != 2 {
		t.Fatalf("dynamic system plan = %#v", planned)
	}
	if planned.Placements[0].Event != SessionStart || !planned.Placements[1].Reevaluate {
		t.Fatalf("dynamic system placements = %#v", planned.Placements)
	}

	degradedStatic := fakeAdapter{name: "claude-code", caps: map[string]map[Capability]bool{
		SessionStart:    {AddContext: true},
		PromptSubmitted: {AddContext: true},
	}}
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{staticRule}}, degradedStatic)
	if err != nil {
		t.Fatal(err)
	}
	planned = report.Rules[0]
	if planned.Status != Degraded || len(planned.Placements) != 1 || planned.Placements[0].Event != SessionStart {
		t.Fatalf("static to dynamic fallback should prefer session.start once: %#v", planned)
	}

	degradedStatic.caps[SessionStart] = nil
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{staticRule}}, degradedStatic)
	if err != nil {
		t.Fatal(err)
	}
	planned = report.Rules[0]
	if planned.Status != Degraded || len(planned.Placements) != 1 || planned.Placements[0].Event != PromptSubmitted ||
		!planned.Placements[0].Reevaluate || planned.Placements[0].Repeat != "session" {
		t.Fatalf("static fallback to prompt.submitted = %#v", planned)
	}

	degradedStatic.slots = map[string]string{"append": ""}
	degradedStatic.caps[SessionStart] = map[Capability]bool{AddContext: true}
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{staticRule}}, degradedStatic)
	if err != nil {
		t.Fatal(err)
	}
	planned = report.Rules[0]
	if planned.Status != Degraded || len(planned.Placements) != 1 || planned.Placements[0].Event != SessionStart {
		t.Fatalf("empty static slot should fall back to a valid dynamic point: %#v", planned)
	}
}

func TestRequirementForTreatsSystemWarnAsDynamic(t *testing.T) {
	rule := rule("system-warn", "system", "warn", "", "degrade", "Review carefully.")
	requirement, err := RequirementFor(rule)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requirement.Events, []string{SessionStart, PromptSubmitted}; !reflect.DeepEqual(got, want) {
		t.Fatalf("system warn events = %#v, want %#v", got, want)
	}
	if got, want := requirement.AnyCapabilities, []Capability{AddContext, SystemMessage}; !reflect.DeepEqual(got, want) {
		t.Fatalf("system warn capabilities = %#v, want %#v", got, want)
	}

	adapter := fakeAdapter{
		name:  "codex",
		slots: map[string]string{"append": "developer_instructions"},
		caps: map[string]map[Capability]bool{
			SessionStart:    {AddContext: true, SystemMessage: true},
			PromptSubmitted: {AddContext: true, SystemMessage: true},
		},
	}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	placements := report.Rules[0].Placements
	if len(placements) != 2 || placements[0].Event != SessionStart || placements[1].Event != PromptSubmitted {
		t.Fatalf("system warn placements = %#v, want two dynamic placements", placements)
	}
	for _, placement := range placements {
		if placement.StaticSlot != "" {
			t.Fatalf("system warn unexpectedly used static location: %#v", placement)
		}
	}
}

func TestBuildSpecialStatusesAndFallbackError(t *testing.T) {
	nativeRule := rule("native", "native", "inject", "", "degrade", "hook")
	providerRule := rule("provider", "tool", "inject", "before", "degrade", "hook")
	providerRule.Effective["when"] = map[string]any{"provider": "openai"}
	blindRule := rule("blind", "tool", "inject", "before", "degrade", "hook")
	errorRule := rule("error", "tool", "inject", "before", "error", "hook")
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{nativeRule, providerRule, blindRule, errorRule}}, fakeAdapter{
		name:    "codex",
		blindID: "blind",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The native rule has no codex mapping, so it is skipped regardless of fallback.
	if report.Rules[0].Status != Skipped || report.Rules[1].Status != Skipped || report.Rules[2].Status != Never {
		t.Fatalf("special statuses = %#v", report.Rules[:3])
	}
	if !report.Blocked || report.Rules[3].Status != InstallError || !report.Rules[3].Fatal {
		t.Fatalf("fallback error = %#v; report blocked=%t", report.Rules[3], report.Blocked)
	}
}

func TestBuildReportsStaticFallbackWithoutRuleBody(t *testing.T) {
	adapter := fakeAdapter{
		name:  "codex",
		slots: map[string]string{"append": "developer_instructions"},
	}
	rule := rule("fallback", "tool", "inject", "before", "degrade", "Use the tool carefully.")
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Degraded || !planned.Placements[0].StaticFallback {
		t.Fatalf("static fallback = %#v", planned)
	}
	rule.Effective["tool"] = "file.edit"
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := report.Rules[0].Placements[0].StaticPrefix, "When using file.edit: "; got != want {
		t.Fatalf("static fallback prefix = %q, want %q", got, want)
	}
	if strings.Contains(report.String(), "Use the tool carefully") {
		t.Fatal("plan report must not include rule body text")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal(report) error = %v", err)
	}
	if strings.Contains(string(encoded), "Use the tool carefully") || strings.Contains(string(encoded), "When using file.edit") {
		t.Fatalf("JSON report leaked rule content or an internal compiler prefix: %s", encoded)
	}
	repeated, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("second Marshal(report) error = %v", err)
	}
	if string(encoded) != string(repeated) {
		t.Fatalf("JSON report is not deterministic:\n%s\n%s", encoded, repeated)
	}
}

func TestBuildUsesAlternateEventWhenConditionDataIsMissing(t *testing.T) {
	rule := rule("added", "tool", "inject", "before", "degrade", "Added text.")
	rule.Effective["when"] = map[string]any{"conditions": []any{map[string]any{
		"field": "tool.added", "op": "contains", "pattern": "TODO",
	}}}
	adapter := fakeAdapter{
		name: "codex",
		caps: map[string]map[Capability]bool{
			ToolPre:  {AddContext: true},
			ToolPost: {AddContext: true},
		},
		data: map[string]map[string]bool{
			ToolPre:  {"tool.added": false},
			ToolPost: {"tool.added": true},
		},
	}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Degraded || len(planned.Placements) != 1 || planned.Placements[0].Event != ToolPost {
		t.Fatalf("alternate event plan = %#v", planned)
	}
}

func TestBuildReportsNeverTriggerWhenAllConditionSourcesAreMissing(t *testing.T) {
	rule := rule("needs-output", "tool", "inject", "before", "degrade", "Output: {{tool.output}}")
	rule.Effective["when"] = map[string]any{"conditions": []any{map[string]any{
		"field": "tool.output", "op": "contains", "pattern": "done",
	}}}
	adapter := fakeAdapter{name: "codex", data: map[string]map[string]bool{
		ToolPre:  {"tool.output": false},
		ToolPost: {"tool.output": false},
	}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Rules[0].Status; got != Never {
		t.Fatalf("status = %q, want %q; report=%s", got, Never, report.String())
	}
}

func TestBuildPlansSkillImplicitAndExplicitPoints(t *testing.T) {
	rule := rule("review", "skill", "inject", "after", "degrade", "Use the review skill.")
	rule.Effective["skill"] = "review"
	adapter := fakeAdapter{name: "claude-code", caps: map[string]map[Capability]bool{
		ToolPost:      {AddContext: true},
		SkillExplicit: {AddContext: true},
	}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Native || len(planned.Placements) != 2 {
		t.Fatalf("skill plan = %#v", planned)
	}
	if planned.Placements[0].Event != ToolPost || planned.Placements[1].Event != SkillExplicit {
		t.Fatalf("skill placements = %#v", planned.Placements)
	}
}

func TestBuildFallbackSkipDropsSkillWhenOnePlacementIsUnavailable(t *testing.T) {
	rule := rule("review", "skill", "inject", "after", "skip", "Use the review skill.")
	rule.Effective["skill"] = "review"
	adapter := fakeAdapter{name: "claude-code", caps: map[string]map[Capability]bool{
		ToolPost: {AddContext: true},
	}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Skipped || len(planned.Placements) != 0 {
		t.Fatalf("fallback:skip left a partial skill rule installed: %#v", planned)
	}
}

func TestBlockNeverUsesStaticInjectFallback(t *testing.T) {
	rule := rule("block", "tool", "block", "before", "degrade", "Do not run this tool.")
	adapter := fakeAdapter{name: "codex", slots: map[string]string{"append": "developer_instructions"}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	planned := report.Rules[0]
	if planned.Status != Skipped || len(planned.Placements) != 0 {
		t.Fatalf("block plan = %#v", planned)
	}
}

func TestBuildFiltersHarnessSelector(t *testing.T) {
	rule := rule("codex-only", "tool", "inject", "before", "degrade", "Codex instruction.")
	rule.Effective["when"] = map[string]any{"harness": map[string]any{"name": "codex"}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, fakeAdapter{name: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Rules[0].Status != Skipped {
		t.Fatalf("status = %q, want skipped", report.Rules[0].Status)
	}
}

func TestBuildWarnsWhenOptionalTemplateValueIsUnavailable(t *testing.T) {
	rule := rule("model-note", "tool", "inject", "before", "degrade", "Model is {{model}}.")
	adapter := fakeAdapter{
		name: "codex",
		caps: map[string]map[Capability]bool{ToolPre: {AddContext: true}},
		data: map[string]map[string]bool{ToolPre: {"model": false}},
	}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	placement := report.Rules[0].Placements[0]
	if len(placement.Warnings) != 1 || !strings.Contains(placement.Warnings[0], "model") {
		t.Fatalf("warnings = %#v", placement.Warnings)
	}
}

func TestBuildFallbackErrorBlocksOnMissingConditionSource(t *testing.T) {
	rule := rule("source", "tool", "inject", "before", "error", "Body")
	rule.Effective["when"] = map[string]any{"conditions": []any{map[string]any{
		"field": "tool.added", "op": "contains", "pattern": "TODO",
	}}}
	adapter := fakeAdapter{name: "codex", caps: map[string]map[Capability]bool{
		ToolPre: {AddContext: true},
	}, data: map[string]map[string]bool{
		ToolPre: {"tool.added": false},
	}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Blocked || report.Rules[0].Status != InstallError || !report.Rules[0].Fatal {
		t.Fatalf("missing condition source did not block plan: %#v", report)
	}
}

func TestBuildRejectsDynamicSystemWithoutUsablePlacement(t *testing.T) {
	rule := rule("dynamic", "system", "inject", "", "degrade", "Model: {{model}}")
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, fakeAdapter{name: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Blocked || report.Rules[0].Status != InstallError {
		t.Fatalf("dynamic system without any placement should be an error: %#v", report)
	}
}

func TestBuildRejectsReplaceWhenStaticLocationIsUnavailable(t *testing.T) {
	rule := rule("replace", "system", "inject", "", "degrade", "Replace all instructions.")
	rule.Effective["mode"] = "replace"
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, fakeAdapter{name: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Rules[0].Status; got != Skipped {
		t.Fatalf("degraded replace status = %q, want skipped", got)
	}
	if len(report.Rules[0].Placements) != 0 || report.Blocked {
		t.Fatalf("degraded replace placements/block = %#v/%t", report.Rules[0].Placements, report.Blocked)
	}

	rule.Effective["fallback"] = "error"
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{rule}}, fakeAdapter{name: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Rules[0].Status != InstallError || !report.Blocked {
		t.Fatalf("error fallback replace plan = %#v", report)
	}
}

func TestBuildLoadsValidatesMergesAndPlansPackage(t *testing.T) {
	root := t.TempDir()
	manifest := `spec: style/v0.3
name: plan-fixture
version: 0.1.0
description: Plan integration fixture
rules:
  - id: static
    target: system
    text: Always review the repository changes.
  - id: shell
    target: tool
    tool: Bash
    at: before
    text: Check shell commands before execution.
`
	if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := spec.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, diagnostic := range spec.Validate(pkg) {
		if diagnostic.Severity == spec.SeverityError {
			t.Fatalf("Validate() error: %s", diagnostic.Error())
		}
	}
	resolved, err := spec.Merge(pkg)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	report, err := Build(resolved, fakeAdapter{
		name:  "codex",
		slots: map[string]string{"append": "developer_instructions"},
		caps:  map[string]map[Capability]bool{ToolPre: {AddContext: true}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if report.Package != "plan-fixture" || report.Adapter != "codex" || len(report.Rules) != 2 {
		t.Fatalf("report identity/rules = %#v", report)
	}
	if report.Rules[0].Status != Native || report.Rules[0].Placements[0].StaticSlot != "developer_instructions" {
		t.Fatalf("static rule plan = %#v", report.Rules[0])
	}
	if report.Rules[1].Status != Native || report.Rules[1].Placements[0].Event != ToolPre {
		t.Fatalf("shell rule plan = %#v", report.Rules[1])
	}
	if strings.Contains(report.String(), "Always review the repository changes") || strings.Contains(report.String(), "Check shell commands") {
		t.Fatalf("human report leaked rule bodies:\n%s", report.String())
	}
}

func rule(id, target, action, at, fallback, body string) spec.Rule {
	effective := map[string]any{
		"id":       id,
		"target":   target,
		"action":   action,
		"fallback": fallback,
		"repeat":   "session",
	}
	if at != "" {
		effective["at"] = at
	}
	return spec.Rule{Source: "STYLE.yaml#" + id, Fields: map[string]any{"id": id}, Effective: effective, Body: body}
}

func packageWithName(name string) *spec.Package {
	return &spec.Package{Metadata: map[string]any{"name": name}}
}
