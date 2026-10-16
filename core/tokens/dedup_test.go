package tokens

import (
	"reflect"
	"testing"

	"github.com/inxups/style-cli/core/plan"
)

func TestDeduplicatedRowsKeepSourceBudgetsButHaveNoEffectiveCost(t *testing.T) {
	adapter := fakeAdapter{static: true, caps: map[string]map[plan.Capability]bool{
		plan.SessionStart: {plan.AddContext: true}, plan.PromptSubmitted: {plan.AddContext: true},
	}}
	pkg, planned, source := loadPlan(t, "budget: {per_rule: 1, per_event: 1, always_on: 1, on: error}\nreads: {model: true}\n", `  - {id: static, target: system, text: Long static text.}
  - {id: dynamic, target: system, text: 'Model {{model}}.'}
`, adapter)
	original, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	for i := range planned.Rules {
		planned.Rules[i].DeduplicatedInto = "other/" + planned.Rules[i].ID
	}
	effective, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if effective.AlwaysOn != (Estimate{}) || len(effective.Events) != 0 || !reflect.DeepEqual(effective.Diagnostics, original.Diagnostics) {
		t.Fatalf("source budgets changed or alias cost remains: original=%+v effective=%+v", original, effective)
	}
	codes := map[string]bool{}
	for _, finding := range effective.Diagnostics {
		if finding.Severity == "error" {
			codes[finding.Code] = true
		}
	}
	for _, code := range []string{"T001", "T002", "T004"} {
		if !codes[code] {
			t.Fatalf("missing source budget %s: %+v", code, effective.Diagnostics)
		}
	}
	for i, rule := range effective.Rules {
		if rule.Estimate != original.Rules[i].Estimate || rule.Estimate.Tokens == 0 || rule.DeduplicatedInto != "other/"+rule.RuleID {
			t.Fatalf("lost original row estimate or alias: %+v", rule)
		}
	}
}

func TestDedupInstalledBudgetRecountsSeparatorsAndBlockBranch(t *testing.T) {
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{
		plan.ToolPre: {plan.AddContext: true, plan.Deny: true},
	}}
	pkg, planned, source := loadPlan(t, "", `  - {id: keep, target: tool, tool: shell, at: before, text: aaaa}
  - {id: alias, target: tool, tool: shell, at: before, text: aaaa}
  - {id: block, target: tool, tool: shell, at: before, action: block, text: bbbbbbbb}
  - {id: alias-block, target: tool, tool: shell, at: before, action: block, text: bbbbbbbb}
`, adapter)
	// In real compilation aliases come from other packages. This plan isolates
	// the accounting boundary: one inject plus one block remains, max(1,2)=2.
	planned.Rules[1].DeduplicatedInto = "other/keep"
	planned.Rules[3].DeduplicatedInto = "other/block"
	report, err := Analyze(pkg, planned, source...)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 1 || report.Events[0].Estimate != (Estimate{Tokens: 2, Chars: 8}) || report.Events[0].BlockCount != 1 || report.Events[0].OtherCount != 1 {
		t.Fatalf("retained alias separator/branch: %+v", report.Events)
	}
	// Add a second independently retained inject: (4+4+2) chars => 10,
	// (1+1+1) tokens => 3, which now exceeds the block side's 2 tokens.
	other := Report{Events: []EventEstimate{{Key: plan.ToolPre, Other: Estimate{Tokens: 1, Chars: 4}, OtherCount: 1}}}
	for _, input := range []InputLimit{{Value: 4, Unit: TokensUnit}, {Value: 13, Unit: CharsUnit}} {
		findings, err := AnalyzeInstalled([]Report{report, other}, input)
		if err != nil || len(findings) != 0 {
			t.Fatalf("legal effective output: %v %v", findings, err)
		}
	}
	for _, input := range []InputLimit{{Value: 3, Unit: TokensUnit}, {Value: 12, Unit: CharsUnit}} {
		findings, err := AnalyzeInstalled([]Report{report, other}, input)
		if err != nil || len(findings) != 1 || findings[0].Code != "T003" {
			t.Fatalf("missing real overflow: %v %v", findings, err)
		}
	}
}
