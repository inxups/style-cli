package plan

import (
	"slices"
	"testing"

	"github.com/inxups/style-cli/core/spec"
)

func TestM3A11ProviderPlanUsesExplicitDataSource(t *testing.T) {
	r := rule("service", "tool", "inject", "before", "degrade", "Service {{provider}}")
	r.Effective["when"] = map[string]any{"provider": []any{"openai", "custom.ID"}}
	req, err := RequirementFor(r)
	if err != nil || !slices.Contains(req.Needs, "provider") || !slices.Contains(req.ConditionNeeds, "provider") {
		t.Fatal(req, err)
	}
	a := fakeAdapter{name: "test", caps: map[string]map[Capability]bool{ToolPre: {AddContext: true}}}
	report, err := Build(&spec.ResolvedPackage{Package: packageWithName("p"), Rules: []spec.Rule{r}}, a)
	if err != nil || report.Rules[0].Status != Native || len(report.Rules[0].Placements) != 1 {
		t.Fatal(report, err)
	}
	a.data = map[string]map[string]bool{ToolPre: {"provider": false}, ToolPost: {"provider": false}}
	report, err = Build(&spec.ResolvedPackage{Package: packageWithName("p"), Rules: []spec.Rule{r}}, a)
	if err != nil || report.Rules[0].Status != Never {
		t.Fatal(report, err)
	}
}
