package codex

import (
	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks"
	"testing"
)

func TestM3A1OfflineEnricherNeverReadsConfigurationOrAncestors(t *testing.T) {
	raw := "high"
	enricher := NewEnricher()
	enricher.Offline = true
	enricher.OfflineConfigEffort = &raw
	enricher.ReadFile = func(string) ([]byte, error) { t.Fatal("external file read"); return nil, nil }
	enricher.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) {
		t.Fatal("process ancestry read")
		return agenthooks.CodexLaunchSettings{}, false
	}
	enricher.ProjectTrusted = func(string) bool { t.Fatal("external trust read"); return false }
	event := &agenthooks.Event{Provider: agenthooks.ProviderCodex, Session: agenthooks.SessionInfo{ID: "s", CWD: "/foreign"}}
	update := enricher.StateUpdate(event, event, state.Session{})
	if update.ThinkingEffort != "high" || !update.EffortResolved {
		t.Fatalf("update %+v", update)
	}
	previous := state.Session{EffortResolved: true, ThinkingEffort: "low", EffortRaw: "low"}
	update = enricher.StateUpdate(event, event, previous)
	if update.ThinkingEffort != "" || update.EffortResolved {
		t.Fatalf("overrode cached effort %+v", update)
	}
}
