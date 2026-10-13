package tokens

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

type fakeAdapter struct {
	caps   map[string]map[plan.Capability]bool
	static bool
}

func (a fakeAdapter) Name() string { return "fixture" }
func (a fakeAdapter) Supports(event string, capability plan.Capability) bool {
	return a.caps[event][capability]
}
func (fakeAdapter) DataSource(string, string) bool         { return true }
func (fakeAdapter) BlindSpot(spec.Rule) (string, bool)     { return "", false }
func (a fakeAdapter) StaticLocation(string) (string, bool) { return "instructions", a.static }

func loadPlan(t *testing.T, metadata, rules string, adapter fakeAdapter) (*spec.ResolvedPackage, plan.Report, []spec.Diagnostic) {
	t.Helper()
	manifest := "spec: style/v0.3\nname: token-fixture\nversion: 0.1.0\ndescription: Token fixture\n" + metadata + "rules:\n" + rules
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := spec.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := spec.Validate(pkg)
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == spec.SeverityError && !strings.HasPrefix(diagnostic.Code, "T00") {
			t.Fatalf("invalid test fixture: %s", diagnostic)
		}
	}
	resolved, err := spec.Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := plan.Build(resolved, adapter)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, planned, diagnostics
}

func TestAnalyzeAlwaysOnAndDynamicPlaceholders(t *testing.T) {
	rules := "  - id: static\n    target: system\n    text: abcd\n  - id: dynamic\n    target: system\n    text: '{{model}}'\n"
	adapter := fakeAdapter{static: true, caps: map[string]map[plan.Capability]bool{
		plan.SessionStart: {plan.AddContext: true}, plan.PromptSubmitted: {plan.AddContext: true},
	}}
	pkg, planned, source := loadPlan(t, "reads: {model: true}\n", rules, adapter)
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := report.AlwaysOn, (Estimate{Tokens: 17, Chars: 68}); got != want {
		t.Fatalf("AlwaysOn = %#v, want %#v", got, want)
	}
	if got, want := report.Rules[1].Estimate, (Estimate{Tokens: 16, Chars: 64}); got != want {
		t.Fatalf("dynamic estimate = %#v, want %#v", got, want)
	}
	if len(report.Events) != 2 || report.Events[0].Key != plan.PromptSubmitted || report.Events[1].Key != plan.SessionStart {
		t.Fatalf("events = %#v", report.Events)
	}
}

func TestAnalyzeWarnCountsModelTextAndIgnoresUserOnlyOutput(t *testing.T) {
	rules := "  - id: default-warn\n    target: tool\n    tool: shell\n    action: warn\n    text: 'OK. " + strings.Repeat("界", 80) + "'\n  - id: explicit-warn\n    target: tool\n    tool: shell\n    action: warn\n    text: 'Long full warning'\n    model_text: abcd\n"
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{
		plan.ToolPost: {plan.AddContext: true, plan.SystemMessage: true},
	}}
	pkg, planned, source := loadPlan(t, "", rules, adapter)
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rules[0].Estimate != EstimateText("OK.") || report.Rules[1].Estimate != EstimateText("abcd") {
		t.Fatalf("warn estimates = %#v", report.Rules)
	}
	adapter.caps[plan.ToolPost] = map[plan.Capability]bool{plan.SystemMessage: true}
	planned, err = plan.Build(pkg, adapter)
	if err != nil {
		t.Fatal(err)
	}
	report, err = Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 0 || report.Rules[0].Estimate != (Estimate{}) {
		t.Fatalf("user-only warning should spend zero model input: %#v", report)
	}
}

func TestAnalyzeT001T002T004SeverityAndBoundary(t *testing.T) {
	metadata := "budget: {per_rule: 1, per_event: 2, always_on: 1, on: error}\n"
	rules := "  - id: static\n    target: system\n    text: abcde\n  - id: one\n    target: event\n    event: prompt.submitted\n    text: abcd\n  - id: two\n    target: event\n    event: prompt.submitted\n    text: abcd\n"
	adapter := fakeAdapter{static: true, caps: map[string]map[plan.Capability]bool{plan.PromptSubmitted: {plan.AddContext: true}}}
	pkg, planned, source := loadPlan(t, metadata, rules, adapter)
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, diagnostic := range report.Diagnostics {
		got = append(got, diagnostic.Code)
		if diagnostic.Severity != "error" {
			t.Fatalf("%s severity = %s", diagnostic.Code, diagnostic.Severity)
		}
	}
	if !reflect.DeepEqual(got, []string{"T001", "T002", "T004"}) {
		t.Fatalf("diagnostic codes = %#v", got)
	}
}

func TestAnalyzeUsesWarningSeverityByDefault(t *testing.T) {
	rules := "  - id: too-large\n    target: system\n    text: abcde\n"
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{
		plan.SessionStart: {plan.AddContext: true}, plan.PromptSubmitted: {plan.AddContext: true},
	}}
	pkg, planned, _ := loadPlan(t, "budget: {per_rule: 1, per_event: 1, always_on: 1}\n", rules, adapter)
	// Omitting source diagnostics asks Analyze to run shared package lints.
	report, err := Analyze(pkg, planned)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Severity != spec.SeverityWarning {
			t.Fatalf("default budget severity = %q, want warning", diagnostic.Severity)
		}
	}
	if len(report.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v, want T001, T002, T004", report.Diagnostics)
	}
}

type mergedLayout struct{}

func (mergedLayout) Buckets(_ spec.Rule, placement plan.Placement) []Bucket {
	if placement.Event == plan.PromptSubmitted || placement.Event == plan.SkillExplicit {
		return []Bucket{{Key: "UserPromptSubmit", Label: "UserPromptSubmit"}}
	}
	return []Bucket{{Key: "PostToolUse Bash", Label: "PostToolUse Bash"}, {Key: "PostToolUse Bash", Label: "duplicate"}}
}

func TestAnalyzeMergesNativeHookPointsAndDeduplicatesBuckets(t *testing.T) {
	rules := "  - id: prompt\n    target: event\n    event: prompt.submitted\n    text: abcd\n  - id: skill\n    target: skill\n    skill: review\n    text: abcd\n"
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{
		plan.PromptSubmitted: {plan.AddContext: true}, plan.SkillExplicit: {plan.AddContext: true}, plan.ToolPost: {plan.AddContext: true},
	}}
	pkg, planned, source := loadPlan(t, "", rules, adapter)
	report, err := AnalyzeWithLayout(pkg, planned, mergedLayout{}, source...)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 2 || report.Events[0].Key != "PostToolUse Bash" || report.Events[0].Estimate != (Estimate{Tokens: 1, Chars: 4}) || report.Events[1].Key != "UserPromptSubmit" || report.Events[1].Estimate != (Estimate{Tokens: 3, Chars: 10}) {
		t.Fatalf("native buckets = %#v", report.Events)
	}
}

func TestAnalyzeBlockAndNonBlockTakeWorstCaseAndT003(t *testing.T) {
	rules := "  - id: inject\n    target: tool\n    tool: shell\n    at: before\n    text: abcd\n  - id: block\n    target: tool\n    tool: shell\n    action: block\n    text: abcdefgh\n"
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{plan.ToolPre: {plan.AddContext: true, plan.Deny: true}}}
	pkg, planned, source := loadPlan(t, "", rules, adapter)
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := report.Events[0].Estimate, (Estimate{Tokens: 2, Chars: 8}); got != want {
		t.Fatalf("block plus inject estimate = %#v, want %#v", got, want)
	}
	findings, err := AnalyzeInstalled([]Report{report, report}, InputLimit{Value: 2, Unit: TokensUnit})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Code != "T003" {
		t.Fatalf("cross-package estimate should trigger T003: %#v", findings)
	}
}

func TestAnalyzeUsesOutputSpecificSeparators(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   Estimate
	}{
		{action: "block", want: EstimateText("abcd\nabcd")},
		{action: "inject", want: EstimateText("abcd\n\nabcd")},
	} {
		t.Run(tc.action, func(t *testing.T) {
			rules := "  - id: first\n    target: tool\n    tool: shell\n    action: " + tc.action + "\n    at: before\n    text: abcd\n  - id: second\n    target: tool\n    tool: shell\n    action: " + tc.action + "\n    at: before\n    text: abcd\n"
			adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{
				plan.ToolPre: {plan.AddContext: true, plan.Deny: true},
			}}
			pkg, planned, source := loadPlan(t, "", rules, adapter)
			report, err := Analyze(pkg, planned, source...)
			if err != nil {
				t.Fatal(err)
			}
			if got := report.Events[0].Estimate; got != tc.want {
				t.Fatalf("combined estimate = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestAnalyzeInstalledChecksEightyPercentInBothUnits(t *testing.T) {
	report := Report{Events: []EventEstimate{{Key: "PreToolUse", Label: "PreToolUse", Estimate: Estimate{Tokens: 80, Chars: 800}, Other: Estimate{Tokens: 80, Chars: 800}, OtherCount: 1}}}
	for _, input := range []InputLimit{{Value: 100, Unit: TokensUnit}, {Value: 1000, Unit: CharsUnit}} {
		findings, err := AnalyzeInstalled([]Report{report}, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 0 {
			t.Fatalf("exactly 80 percent must pass: %#v", findings)
		}
		findings, err = AnalyzeInstalled([]Report{report, report}, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Severity != "error" {
			t.Fatalf("two installed packages should trigger T003: %#v", findings)
		}
	}
	if _, err := AnalyzeInstalled(nil, InputLimit{Value: 100, Unit: "bytes"}); err == nil {
		t.Fatal("unsupported unit accepted")
	}
}

func TestAnalyzeInstalledUsesStableEventLabels(t *testing.T) {
	first := Report{Events: []EventEstimate{{Key: "PreToolUse", Label: "Z event", Estimate: Estimate{Tokens: 81}, Other: Estimate{Tokens: 81}, OtherCount: 1}}}
	second := Report{Events: []EventEstimate{{Key: "PreToolUse", Label: "A event", Estimate: Estimate{Tokens: 81}, Other: Estimate{Tokens: 81}, OtherCount: 1}}}
	for _, reports := range [][]Report{{first, second}, {second, first}} {
		findings, err := AnalyzeInstalled(reports, InputLimit{Value: 100, Unit: TokensUnit})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Event != "A event" {
			t.Fatalf("findings depend on report order: %#v", findings)
		}
	}
}

func TestAnalyzePropagatesSharedT005T006T007AndStableJSON(t *testing.T) {
	metadata := "reads: {harness: [session_id]}\n"
	rules := "  - id: blocker\n    target: tool\n    tool: shell\n    action: block\n    text: '" + strings.Repeat("界", 41) + "'\n  - id: repeated\n    target: event\n    event: prompt.submitted\n    repeat: every\n    text: '{{harness.session_id}}'\n"
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{plan.ToolPre: {plan.Deny: true}, plan.PromptSubmitted: {plan.AddContext: true}}}
	pkg, planned, source := loadPlan(t, metadata, rules, adapter)
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, diagnostic := range report.Diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	if !reflect.DeepEqual(codes, []string{"T005", "T006", "T007"}) {
		t.Fatalf("source lint propagation = %#v", report.Diagnostics)
	}
	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	reportAgain, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(reportAgain)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("repeated analysis JSON changed")
	}
	if strings.Contains(string(first), strings.Repeat("界", 41)) {
		t.Fatal("JSON report leaked text")
	}
}

func TestAnalyzeStaticFallbackIncludesQualifiedPrefix(t *testing.T) {
	rules := "  - id: fallback\n    target: tool\n    tool: shell\n    text: abcd\n"
	pkg, planned, source := loadPlan(t, "", rules, fakeAdapter{static: true})
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := report.AlwaysOn, EstimateText("When using shell: abcd"); got != want {
		t.Fatalf("static fallback estimate = %#v, want %#v", got, want)
	}
	if len(report.Events) != 0 {
		t.Fatalf("static fallback should not be a runtime event: %#v", report.Events)
	}
}

func TestAnalyzeExcludesPromptBlockReasonFromModelInput(t *testing.T) {
	rules := "  - id: block-prompt\n    target: event\n    event: prompt.submitted\n    action: block\n    text: abcdefgh\n"
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{plan.PromptSubmitted: {plan.Deny: true}}}
	pkg, planned, source := loadPlan(t, "", rules, adapter)
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rules[0].Estimate != (Estimate{}) {
		t.Fatalf("prompt block reason counted as model input: %#v", report.Rules[0])
	}
	if len(report.Events) != 1 || !report.Events[0].HasBlock || report.Events[0].Estimate != (Estimate{}) {
		t.Fatalf("prompt block metadata = %#v", report.Events)
	}
}
