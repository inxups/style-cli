package conformance_test

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

type effortFixture struct {
	Raw    string `json:"raw"`
	Tier   string `json:"tier"`
	Approx *bool  `json:"approx"`
}

func TestEveryEmbeddedEffortMappingThroughRuntime(t *testing.T) {
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			var fixtures []effortFixture
			if err := json.Unmarshal(read(t, filepath.Join(corpus, string(suite.id)+"-effort.json")), &fixtures); err != nil {
				t.Fatal(err)
			}
			table, err := codex.EffortTable()
			if suite.id == agenthooks.ProviderClaudeCode {
				table, err = claude.EffortTable()
			}
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			index := buildIndex(t, suite, read(t, filepath.Join(corpus, "STYLE.yaml")))
			for _, fixture := range fixtures {
				t.Run(fixture.Raw, func(t *testing.T) {
					seen[fixture.Raw] = true
					configured := suite
					payload := map[string]any{"session_id": "effort", "cwd": "/work/repo", "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]string{"command": "true"}}
					if suite.id == agenthooks.ProviderCodex {
						configured.enricher = func(t *testing.T) adapter.RuntimeEnricher {
							result := suite.enricher(t).(*codex.Enricher)
							result.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) {
								return agenthooks.CodexLaunchSettings{ModelReasoningEffort: fixture.Raw, IgnoreUserConfig: true}, true
							}
							return result
						}
					} else {
						payload["effort"] = map[string]string{"level": fixture.Raw}
					}
					raw, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					store := newStore(t)
					_, projection := invoke(t, configured, index, raw, store)
					session, err := store.ReadSession("effort")
					if err != nil {
						t.Fatal(err)
					}
					checkEffort(t, fixture, session, projection.Variables)
				})
			}
			for _, raw := range table.Exact {
				if !seen[raw] {
					t.Errorf("missing exact effort fixture %q", raw)
				}
			}
			for raw := range table.Aliases {
				if !seen[raw] {
					t.Errorf("missing alias fixture %q", raw)
				}
			}
		})
	}
}

func checkEffort(t *testing.T, fixture effortFixture, session state.Session, variables map[string]string) {
	t.Helper()
	if !session.EffortResolved || session.ThinkingEffort != fixture.Tier || session.EffortRaw != fixture.Raw || variables["thinking_effort"] != fixture.Tier || variables["thinking_effort_raw"] != fixture.Raw {
		t.Fatalf("effort %q: state=%#v variables=%#v", fixture.Raw, session, variables)
	}
	if fixture.Approx == nil {
		if session.EffortApprox != nil || variables["thinking_effort_approx"] != "" {
			t.Fatal("unknown effort fabricated an approximation")
		}
	} else if session.EffortApprox == nil || *session.EffortApprox != *fixture.Approx || variables["thinking_effort_approx"] != strconv.FormatBool(*fixture.Approx) {
		t.Fatal("wrong approximation flag")
	}
}
