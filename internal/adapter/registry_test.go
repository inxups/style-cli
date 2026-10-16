package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/speakeasy-api/agenthooks"
)

type testFacts struct {
	capabilities map[string]map[plan.Capability]CapabilitySupport
	sources      map[string]map[string]DataAvailability
	blind        map[string]string
	static       map[string]StaticLocation
}

func (facts testFacts) Capability(point string, capability plan.Capability) CapabilitySupport {
	return facts.capabilities[point][capability]
}

func (facts testFacts) DataSource(point, field string) DataAvailability {
	return facts.sources[point][field]
}

func (facts testFacts) BlindSpot(rule spec.Rule) (string, bool) {
	reason, ok := facts.blind[rule.Fields["id"].(string)]
	return reason, ok
}

func (facts testFacts) StaticLocation(mode string) (StaticLocation, bool) {
	location, ok := facts.static[mode]
	return location, ok
}

type testDetector struct {
	installation Installation
	installed    bool
	err          error
}

func (detector testDetector) Detect(context.Context) (Installation, bool, error) {
	return detector.installation, detector.installed, detector.err
}

type testPositions []StaticLocation

func (positions testPositions) StaticPositions(Installation) []StaticLocation {
	return append([]StaticLocation(nil), positions...)
}

type testRoute struct{ name string }

func (route testRoute) Name() string { return route.name }
func (route testRoute) Available(context.Context, Installation) (bool, string, error) {
	return true, "available", nil
}
func (testRoute) Trust() TrustInfo { return TrustInfo{Kind: TrustUserConfirmation, StableKey: true} }
func (route testRoute) Plan(context.Context, []HookSpec, []StaticBlock) (Changeset, error) {
	return Changeset{Files: []FileChange{{Path: route.name + ".json", After: []byte("{}")}}}, nil
}
func (testRoute) Apply(context.Context, Changeset) error           { return nil }
func (testRoute) Remove(context.Context, Owner) (Changeset, error) { return Changeset{}, nil }
func (testRoute) Status(context.Context, Installation) (RouteStatus, error) {
	return RouteStatus{Available: true}, nil
}

func definition(id ProviderID) Definition {
	return Definition{
		ID:              id,
		Detector:        testDetector{},
		Facts:           testFacts{},
		StaticPositions: testPositions{{Name: "instructions", Mode: "append", Supported: true}},
		Routes:          []Route{testRoute{name: "user"}},
		Limits:          Limits{Input: tokens.InputLimit{Value: 2500, Unit: tokens.TokensUnit}, Approximate: true, Source: "provider docs", Overflow: OverflowFilePreview},
		Compact:         CompactSignal{Event: agenthooks.KindSessionStart, SessionSource: "compact", Deferred: true},
	}
}

func TestRegistryIsExplicitAndDeterministic(t *testing.T) {
	z := definition(ProviderID("z-provider"))
	a := definition(ProviderID("a-provider"))
	registry, err := NewRegistry(z, a)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := registry.Providers(), []ProviderID{"a-provider", "z-provider"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Providers() = %#v, want %#v", got, want)
	}
	got, ok := registry.Lookup(a.ID)
	if !ok || got.ID != a.ID || got.Compact.SessionSource != "compact" || got.Limits != a.Limits {
		t.Fatalf("Lookup() = %#v, %t", got, ok)
	}
	if positions := got.StaticPositions.StaticPositions(Installation{Provider: a.ID}); len(positions) != 1 || positions[0].Name != "instructions" {
		t.Fatalf("static positions = %#v", positions)
	}
	got.Routes[0] = testRoute{name: "mutated"}
	again, ok := registry.Lookup(a.ID)
	if !ok || again.Routes[0].Name() != "user" {
		t.Fatalf("lookup route slice mutated registry: %#v", again.Routes)
	}
	if _, ok := registry.Lookup("missing"); ok {
		t.Fatal("Lookup() found an unregistered provider")
	}
	if _, ok := (*Registry)(nil).Lookup("codex"); ok {
		t.Fatal("nil registry unexpectedly found a provider")
	}
}

func TestRegistryRejectsInvalidDefinitions(t *testing.T) {
	valid := definition("fixture")
	for _, tc := range []struct {
		name string
		edit func(*Definition)
	}{
		{name: "empty provider", edit: func(d *Definition) { d.ID = " " }},
		{name: "missing detector", edit: func(d *Definition) { d.Detector = nil }},
		{name: "missing planning facts", edit: func(d *Definition) { d.Facts = nil }},
		{name: "missing static positions", edit: func(d *Definition) { d.StaticPositions = nil }},
		{name: "zero input limit", edit: func(d *Definition) { d.Limits.Input.Value = 0 }},
		{name: "unknown input unit", edit: func(d *Definition) { d.Limits.Input.Unit = "bytes" }},
		{name: "unknown overflow policy", edit: func(d *Definition) { d.Limits.Overflow = "truncate" }},
		{name: "invalid absent compact signal", edit: func(d *Definition) { d.Compact = CompactSignal{SessionSource: "compact"} }},
		{name: "nil route", edit: func(d *Definition) { d.Routes = []Route{nil} }},
		{name: "empty route name", edit: func(d *Definition) { d.Routes = []Route{testRoute{}} }},
		{name: "duplicate routes", edit: func(d *Definition) { d.Routes = []Route{testRoute{"user"}, testRoute{"user"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.edit(&candidate)
			if _, err := NewRegistry(candidate); err == nil {
				t.Fatal("NewRegistry() accepted an invalid definition")
			}
		})
	}
	if _, err := NewRegistry(valid, valid); err == nil {
		t.Fatal("NewRegistry() accepted duplicate provider IDs")
	}
	if _, err := NewRegistry(); err != nil {
		t.Fatalf("empty registry should be valid: %v", err)
	}
}

func TestPlanAdapterProjectsProviderFacts(t *testing.T) {
	facts := testFacts{
		capabilities: map[string]map[plan.Capability]CapabilitySupport{
			plan.ToolPre: {plan.AddContext: {Available: true}},
		},
		sources: map[string]map[string]DataAvailability{
			plan.ToolPre:  {"agent": {Available: true}, "tool.input.command": {Available: true, Source: "stdin", Approximate: true}},
			plan.ToolPost: {"agent": {Available: true}, "tool.input.command": {Available: true, Source: "stdin", Approximate: true}},
		},
		static: map[string]StaticLocation{
			"append": {Name: "developer_instructions", Mode: "append", Supported: true},
		},
	}
	adapter, err := BuildPlanAdapter("codex", facts)
	if err != nil {
		t.Fatal(err)
	}
	rule := spec.Rule{Source: "STYLE.yaml#review", Body: "Check command", Fields: map[string]any{"id": "review"}, Effective: map[string]any{
		"id": "review", "target": "tool", "tool": "shell", "at": "before", "action": "inject",
		"when": map[string]any{"conditions": []any{map[string]any{"field": "tool.input.command", "op": "contains", "pattern": "git"}}},
	}}
	report, err := plan.Build(&spec.ResolvedPackage{Package: &spec.Package{Metadata: map[string]any{"name": "demo"}}, Rules: []spec.Rule{rule}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if report.Adapter != "codex" || len(report.Rules[0].Placements) != 1 || report.Rules[0].Placements[0].Event != plan.ToolPre {
		t.Fatalf("plan through provider facts = %#v", report)
	}
	if got := facts.DataSource(plan.ToolPre, "tool.input.command"); !got.Available || got.Source != "stdin" || !got.Approximate {
		t.Fatalf("data-source detail = %#v", got)
	}
	if got, ok := facts.StaticLocation("append"); !ok || !got.Supported || got.Name != "developer_instructions" {
		t.Fatalf("static location = %#v, %t", got, ok)
	}
}

func TestDefinitionRetainsRoutePriorityAndDetectorError(t *testing.T) {
	definition := definition("fixture")
	definition.Routes = []Route{testRoute{"plugin"}, testRoute{"user"}}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := registry.Lookup(definition.ID)
	if got.Routes[0].Name() != "plugin" || got.Routes[1].Name() != "user" {
		t.Fatalf("route order changed: %#v", got.Routes)
	}

	definition.Detector = testDetector{installation: Installation{Version: "1.2"}, installed: true}
	installation, installed, err := definition.Detect(context.Background())
	if err != nil || !installed || installation.Provider != definition.ID || installation.Version != "1.2" {
		t.Fatalf("Detect() = %#v, %t, %v", installation, installed, err)
	}
	definition.Detector = testDetector{installation: Installation{Provider: "other"}, installed: true}
	if _, _, err := definition.Detect(context.Background()); err == nil {
		t.Fatal("Detect() accepted a mismatched provider ID")
	}

	wantErr := errors.New("detect failed")
	definition.Detector = testDetector{err: wantErr}
	_, _, err = definition.Detect(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Detect() error = %v, want wrapped %v", err, wantErr)
	}

	available, reason, err := definition.Routes[0].Available(context.Background(), installation)
	if err != nil || !available || reason != "available" {
		t.Fatalf("Route.Available() = %t, %q, %v", available, reason, err)
	}
	changes, err := definition.Routes[0].Plan(context.Background(), []HookSpec{{NativeEvent: "PreToolUse", Points: []string{plan.ToolPre}}}, nil)
	if err != nil || len(changes.Files) != 1 || changes.Files[0].Path != "plugin.json" {
		t.Fatalf("Route.Plan() = %#v, %v", changes, err)
	}
	if err := definition.Routes[0].Apply(context.Background(), changes); err != nil {
		t.Fatalf("Route.Apply() = %v", err)
	}
	if removed, err := definition.Routes[0].Remove(context.Background(), Owner("style")); err != nil || len(removed.Files) != 0 {
		t.Fatalf("Route.Remove() = %#v, %v", removed, err)
	}
	if status, err := definition.Routes[0].Status(context.Background(), installation); err != nil || !status.Available {
		t.Fatalf("Route.Status() = %#v, %v", status, err)
	}
}

type nativeTestFacts struct{ testFacts }

func (nativeTestFacts) NativeHook(rule spec.NativeRule) (plan.NativeEntry, error) {
	return plan.NativeEntry{Event: rule.Event, Timeout: 7}, nil
}

func TestPlanAdapterDelegatesNativeHooksToFacts(t *testing.T) {
	planner, err := BuildPlanAdapter("codex", nativeTestFacts{})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := planner.(plan.NativeAdapter).NativeHook(spec.NativeRule{Event: "Stop"})
	if err != nil || entry != (plan.NativeEntry{Event: "Stop", Timeout: 7}) {
		t.Fatalf("NativeHook = %#v, %v", entry, err)
	}
	plain, err := BuildPlanAdapter("codex", testFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.(plan.NativeAdapter).NativeHook(spec.NativeRule{Event: "Stop"}); err == nil || err.Error() != "adapter codex does not support native hooks" {
		t.Fatalf("facts without native support: err = %v", err)
	}
}
