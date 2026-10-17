package tokens

import (
	"reflect"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

func TestM3A0EmptySourceFallbackPreservesStructuralLintsForFakeHarness(t *testing.T) {
	adapter := fakeAdapter{caps: map[string]map[plan.Capability]bool{plan.PromptSubmitted: {plan.AddContext: true}}}
	for _, repeat := range []string{"session", "turn"} {
		t.Run(repeat, func(t *testing.T) {
			rules := "  - id: r\n    target: event\n    event: prompt.submitted\n    repeat: " + repeat + "\n    text: body\n    when: {harness: {name: fixture}}\n"
			pkg, planned, _ := loadPlan(t, "reads: {harness: [name]}\n", rules, adapter)
			source := spec.ValidateWithCatalog(pkg.Package, spec.ValidationCatalog{Harnesses: []string{"fixture"}})
			withSource, err := Analyze(pkg, planned, source...)
			if err != nil {
				t.Fatal(err)
			}
			fallback, err := Analyze(pkg, planned)
			if err != nil || !reflect.DeepEqual(fallback, withSource) {
				t.Fatalf("structural fallback changed the report: %+v != %+v, %v", fallback, withSource, err)
			}
			found := false
			for _, diagnostic := range fallback.Diagnostics {
				found = found || diagnostic.Code == "T007"
			}
			if found != (repeat == "turn") || len(fallback.Events) != 1 || fallback.Events[0].Estimate.Tokens == 0 {
				t.Fatalf("fake budget/lint not evaluated: %+v", fallback)
			}
		})
	}
}
