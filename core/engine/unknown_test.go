package engine

import (
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
)

func TestM3A12UnknownTraceCapRecoveryAndLegacyGate(t *testing.T) {
	entry := testEntry("pkg/missing", "tool.pre", "event", "inject", "Missing data guidance", 1)
	entry.When = when.Rule{Model: []string{"unknown"}, Provider: []string{"unknown"}, ThinkingEffort: []string{"unknown"}, Harness: map[string][]string{"permission_mode": {"unknown"}}, Agent: []string{"main"}}
	entry.Needs = []string{"model", "provider", "thinking_effort", "harness.permission_mode"}
	index := testIndex(map[string][]compile.Entry{"tool.pre": {entry}})
	index.Format = 5
	core, err := New(index, newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	input := testInput("tool.pre")
	input.Variables = map[string]string{}
	input.Points[0].When = when.Input{}
	out, trace, err := core.ProcessWithTrace(input, tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit})
	if err != nil || trace.Entries[0].Output != "cap-drop" || !trace.Entries[0].ReservationReleased {
		t.Fatal(out, trace, err)
	}
	out, trace, err = core.ProcessWithTrace(input, tokens.InputLimit{})
	if err != nil || out.AdditionalContext != "Missing data guidance" {
		t.Fatal(out, trace, err)
	}
	for _, check := range trace.Entries[0].Conditions {
		if check.Field != "agent" && (check.Available || check.Result != "pass" || check.Source != "unavailable") {
			t.Fatal(check)
		}
	}
	input.Scope.SessionID = "known"
	input.Variables["model"] = "present"
	out, _, err = core.ProcessWithTrace(input, tokens.InputLimit{})
	if err != nil || out.AdditionalContext != "" {
		t.Fatal(out, err)
	}
	for _, format := range []int{1, 2, 3, 4} {
		index.Format = format
		if _, err := New(index, newTestStore(t)); err == nil {
			t.Fatal("constructor bypassed unknown gate", format)
		}
	}
}
