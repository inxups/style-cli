package tokens

import (
	"fmt"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/state"
)

func TestRefreshBudgetsUseFullEmission(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, n := range []int{1, 3, 1000000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			rules := fmt.Sprintf("  - {id: r, target: system, text: abcdefghijklmnop, refresh: %d, reason: periodic}\n", n)
			a := fakeAdapter{static: true, caps: map[string]map[plan.Capability]bool{plan.SessionStart: {plan.AddContext: true}, plan.PromptSubmitted: {plan.AddContext: true}}}
			pkg, p, ds := loadPlan(t, "budget: {per_rule: 1, per_event: 1, always_on: 1}\n", rules, a)
			report, err := Analyze(pkg, p, ds...)
			if err != nil {
				t.Fatal(err)
			}
			if report.Rules[0].Estimate != EstimateText("abcdefghijklmnop") {
				t.Fatal(report.Rules)
			}
			codes := map[string]bool{}
			for _, d := range report.Diagnostics {
				codes[d.Code] = true
			}
			if !codes["T001"] || !codes["T002"] {
				t.Fatal(report.Diagnostics)
			}
			findings, err := AnalyzeInstalled([]Report{report}, InputLimit{Value: 2, Unit: TokensUnit})
			if err != nil || len(findings) == 0 || findings[0].Code != "T003" {
				t.Fatal(findings, err)
			}
			// Re-running the same placement classification without refresh yields
			// the same always-on sum; cadence never discounts the body.
			pkg.Rules[0].Effective["refresh"] = 0
			unrefreshed, err := Analyze(pkg, p, ds...)
			if err != nil || report.AlwaysOn != unrefreshed.AlwaysOn {
				t.Fatal(report.AlwaysOn, unrefreshed.AlwaysOn, err)
			}
		})
	}
}
