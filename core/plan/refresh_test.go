package plan

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/state"
)

func TestRefreshSystemRequiresDynamicPrompt(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, tc := range []struct {
		name          string
		start, prompt bool
		fallback      string
		want          Status
		count         int
	}{
		{"dual", true, true, "degrade", Native, 2}, {"prompt", false, true, "degrade", Degraded, 1},
		{"start", true, false, "degrade", Skipped, 0}, {"skip", true, false, "skip", Skipped, 0}, {"error", true, false, "error", InstallError, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := fakeAdapter{name: "fixture", caps: map[string]map[Capability]bool{SessionStart: {AddContext: tc.start}, PromptSubmitted: {AddContext: tc.prompt}}, slots: map[string]string{"append": "developer_instructions"}}
			r := rule("refresh", "system", "inject", "", tc.fallback, "constant body")
			r.Effective["refresh"] = 3
			report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{r}}, a)
			if err != nil {
				t.Fatal(err)
			}
			p := report.Rules[0]
			if p.Status != tc.want || len(p.Placements) != tc.count || p.Refresh == nil {
				t.Fatalf("%+v", p)
			}
			for _, placement := range p.Placements {
				if placement.StaticSlot != "" || placement.Event == PromptSubmitted && !placement.Reevaluate {
					t.Fatal(placement)
				}
			}
			data, _ := json.Marshal(report)
			if !strings.Contains(string(data), `"interval":3`) || !strings.Contains(report.String(), "every 3 submitted turns") {
				t.Fatal(string(data), report.String())
			}
		})
	}
}

func TestRefreshOtherTargetsNeverBecomeStatic(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, target := range []string{"tool", "mcp", "skill", "event"} {
		r := rule("refresh", target, "inject", "after", "degrade", "constant")
		r.Effective["refresh"] = 3
		r.Effective["event"] = ToolError
		a := fakeAdapter{name: "fixture", slots: map[string]string{"append": "developer_instructions"}, caps: map[string]map[Capability]bool{ToolPost: {AddContext: true}, ToolError: {AddContext: true}, SkillExplicit: {AddContext: true}}}
		report, err := Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{r}}, a)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range report.Rules[0].Placements {
			if p.StaticSlot != "" || p.Event == PromptSubmitted {
				t.Fatal(p)
			}
		}
		a.caps = nil
		report, err = Build(&spec.ResolvedPackage{Package: packageWithName("demo"), Rules: []spec.Rule{r}}, a)
		if err != nil || report.Rules[0].Status != Skipped || len(report.Rules[0].Placements) != 0 {
			t.Fatal(report, err)
		}
	}
}
