package claude

import (
	"testing"

	"github.com/inxups/style-cli/core/state"
)

func TestResolveEffortPriorityAndRawValues(t *testing.T) {
	approx := false
	for _, tc := range []struct {
		name       string
		event      string
		env        string
		previous   state.Session
		wantTier   string
		wantRaw    string
		wantSource string
		wantApprox *bool
	}{
		{name: "event wins", event: "XHIGH", env: "low", previous: state.Session{ThinkingEffort: "medium", EffortRaw: "medium", EffortApprox: &approx}, wantTier: "xhigh", wantRaw: "XHIGH", wantSource: "event", wantApprox: &approx},
		{name: "unknown event remains authoritative", event: "future", env: "low", previous: state.Session{ThinkingEffort: "medium", EffortRaw: "medium"}, wantRaw: "future", wantSource: "event"},
		{name: "environment fallback", env: "high", previous: state.Session{ThinkingEffort: "medium", EffortRaw: "medium"}, wantTier: "high", wantRaw: "high", wantSource: "environment", wantApprox: &approx},
		{name: "session fallback", previous: state.Session{ThinkingEffort: "medium", EffortRaw: "custom", EffortApprox: &approx}, wantTier: "medium", wantRaw: "custom", wantSource: "session", wantApprox: &approx},
		{name: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveEffort(tc.event, tc.env, tc.previous)
			if err != nil {
				t.Fatal(err)
			}
			if got.Effort != tc.wantTier || got.EffortRaw != tc.wantRaw || got.Source != tc.wantSource {
				t.Fatalf("ResolveEffort() = %#v, want tier=%q raw=%q source=%q", got, tc.wantTier, tc.wantRaw, tc.wantSource)
			}
			if tc.wantApprox == nil && got.EffortApprox != nil {
				t.Fatalf("unexpected approximation marker: %#v", got.EffortApprox)
			}
			if tc.wantApprox != nil && (got.EffortApprox == nil || *got.EffortApprox != *tc.wantApprox) {
				t.Fatalf("approximation marker = %v, want %v", got.EffortApprox, *tc.wantApprox)
			}
		})
	}
}

func TestResolveCurrentEffortReadsOnlyClaudeEnvironmentFallback(t *testing.T) {
	t.Setenv("CLAUDE_EFFORT", "HIGH")
	got, err := ResolveCurrentEffort("", state.Session{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort != "high" || got.EffortRaw != "HIGH" || got.Source != "environment" || got.EffortApprox == nil || *got.EffortApprox {
		t.Fatalf("ResolveCurrentEffort() = %#v", got)
	}
}
