package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

func TestM3A11ProviderCapRecoveryAndFeatureGate(t *testing.T) {
	entry := testEntry("pkg/service", "tool.pre", "event", "inject", "", 1)
	fallback := strings.Repeat("中", 100)
	entry.Body = compile.Body{Parts: []compile.Fragment{{Variable: &compile.Variable{Name: "provider", Default: &fallback}}}}
	entry.Needs = []string{"provider"}
	entry.When.Provider = []string{"local.ID"}
	entry.When.ThinkingEffortApprox = nil
	index := testIndex(map[string][]compile.Entry{"tool.pre": {entry}})
	index.Format = 4
	delete(index.Harnesses, "claude-code")
	store := newTestStore(t)
	core, err := New(index, store)
	if err != nil {
		t.Fatal(err)
	}
	if !core.NeedsProvider("codex") || core.NeedsProvider("claude-code") {
		t.Fatal("incorrect feature detection")
	}
	input := testInput("tool.pre")
	input.Variables["provider"] = "local.ID"
	input.Points[0].When = when.Input{ProviderSource: "codex-launch"}
	out, trace, err := core.ProcessWithTrace(input, tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit})
	if err != nil || out.AdditionalContext != "" || trace.Entries[0].Output != "cap-drop" || !trace.Entries[0].ReservationReleased {
		t.Fatal(out, trace, err)
	}
	out, trace, err = core.ProcessWithTrace(input, tokens.InputLimit{})
	if err != nil || out.AdditionalContext != "local.ID" {
		t.Fatal(out, err)
	}
	if trace.Entries[0].Conditions[0].Source != "codex-launch" {
		t.Fatal(trace)
	}
	data, _ := json.Marshal(trace)
	if strings.Contains(string(data), "local.ID") {
		t.Fatal("trace exposed service value")
	}
	for _, value := range []string{"a" + strings.Repeat(".", 63), fallback} {
		estimate, err := tokenestimate.EstimateTemplate("{{provider | default: \""+fallback+"\"}}", tokenestimate.Context{})
		actual := tokens.EstimateText(value)
		if err != nil || estimate.Tokens < actual.Tokens || estimate.Chars < actual.Chars {
			t.Fatal(estimate, actual, err)
		}
	}
	for _, version := range []int{1, 2, 3} {
		index.Format = version
		if _, err := New(index, store); err == nil {
			t.Fatal("new constructor bypassed format gate", version)
		}
	}
	old := testIndex(map[string][]compile.Entry{"tool.pre": {testEntry("pkg/old", "tool.pre", "event", "inject", "old", 1)}})
	oldCore, err := New(old, store)
	if err != nil || oldCore.NeedsProvider("codex") {
		t.Fatal(err)
	}
}
