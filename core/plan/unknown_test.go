package plan

import (
	"slices"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/spec"
)

func TestM3A12UnknownPlansDeliberateMissingData(t *testing.T) {
	for _, field := range []string{"model", "provider", "thinking_effort", "harness.permission_mode"} {
		t.Run(field, func(t *testing.T) {
			r := rule("missing", "tool", "inject", "before", "degrade", "Missing guidance.")
			condition := map[string]any{field: []any{"known", "unknown"}}
			if field == "thinking_effort" {
				condition[field] = []any{"high", "unknown"}
			}
			if field == "harness.permission_mode" {
				condition = map[string]any{"harness": map[string]any{"permission_mode": []any{"default", "unknown"}}}
			}
			r.Effective["when"] = condition
			req, err := RequirementFor(r)
			if err != nil || !slices.Contains(req.Needs, field) || slices.Contains(req.ConditionNeeds, field) || !slices.Contains(req.UnknownNeeds, field) {
				t.Fatal(req, err)
			}
			a := fakeAdapter{name: "test", caps: map[string]map[Capability]bool{ToolPre: {AddContext: true}}, data: map[string]map[string]bool{ToolPre: {field: false}, ToolPost: {field: false}}}
			report, err := Build(&spec.ResolvedPackage{Package: packageWithName("p"), Rules: []spec.Rule{r}}, a)
			if err != nil || report.Rules[0].Status != Native {
				t.Fatal(report, err)
			}
			if !strings.Contains(strings.Join(report.Rules[0].Placements[0].Warnings, " "), "explicit unknown selector may match") {
				t.Fatal(report)
			}
			if field == "harness.permission_mode" {
				condition["harness"] = map[string]any{"permission_mode": "default"}
			} else {
				condition[field] = "high"
			}
			report, err = Build(&spec.ResolvedPackage{Package: packageWithName("p"), Rules: []spec.Rule{r}}, a)
			if err != nil || report.Rules[0].Status != Never {
				t.Fatal("ordinary missing data changed", report, err)
			}
		})
	}
}
