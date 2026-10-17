package claude

import (
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks"
)

func TestM3A11ClaudeProviderSelectors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		env          map[string]string
		want, source string
	}{
		{"default", nil, "anthropic", "claude-default"},
		{"bedrock", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}, "bedrock", "claude-environment"},
		{"vertex", map[string]string{"CLAUDE_CODE_USE_VERTEX": "true"}, "vertex", "claude-environment"},
		{"foundry", map[string]string{"CLAUDE_CODE_USE_FOUNDRY": "1"}, "foundry", "claude-environment"},
		{"off", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "0", "CLAUDE_CODE_USE_VERTEX": "false", "CLAUDE_CODE_USE_FOUNDRY": ""}, "anthropic", "claude-default"},
		{"conflict", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1", "CLAUDE_CODE_USE_VERTEX": "1"}, "", "claude-provider-conflicting-selectors"},
		{"invalid", map[string]string{"CLAUDE_CODE_USE_FOUNDRY": "perhaps"}, "", "claude-provider-invalid-selector"},
		{"endpoint", map[string]string{"ANTHROPIC_BASE_URL": "https://private.invalid/key", "CLAUDE_CODE_USE_BEDROCK": "1"}, "", "claude-provider-managed-or-endpoint"},
		{"managed", map[string]string{"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST": "1"}, "", "claude-provider-managed-or-endpoint"},
		{"legacy managed", map[string]string{"CLAUDE_CODE_HOST_MANAGED": "1"}, "", "claude-provider-managed-or-endpoint"},
		{"bedrock endpoint", map[string]string{"ANTHROPIC_BEDROCK_BASE_URL": "private"}, "", "claude-provider-managed-or-endpoint"},
		{"vertex endpoint", map[string]string{"ANTHROPIC_VERTEX_BASE_URL": "private"}, "", "claude-provider-managed-or-endpoint"},
		{"foundry endpoint", map[string]string{"ANTHROPIC_FOUNDRY_BASE_URL": "private"}, "", "claude-provider-managed-or-endpoint"},
		{"AWS endpoint", map[string]string{"ANTHROPIC_AWS_BASE_URL": "private"}, "", "claude-provider-managed-or-endpoint"},
		{"mantle", map[string]string{"CLAUDE_CODE_USE_MANTLE": "1"}, "", "claude-provider-unsupported-route"},
		{"new mantle", map[string]string{"ANTHROPIC_AWS_USE_MANTLE": "1"}, "", "claude-provider-unsupported-route"},
		{"AWS", map[string]string{"CLAUDE_CODE_USE_ANTHROPIC_AWS": "1"}, "", "claude-provider-unsupported-route"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveProvider(func(key string) (string, bool) {
				if strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN") {
					t.Fatal("credential access")
				}
				value, ok := tc.env[key]
				return value, ok
			})
			if got.ID != tc.want || got.Source != tc.source {
				t.Fatal(got)
			}
			if strings.Contains(got.Source, "private") {
				t.Fatal("endpoint leaked")
			}
		})
	}
}
func TestM3A11ClaudeProviderReevaluatesAndClears(t *testing.T) {
	e := NewEnricher()
	calls := 0
	env := map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}
	e.ProviderEnvironment = func(key string) (string, bool) { calls++; v, ok := env[key]; return v, ok }
	event := &agenthooks.Event{Provider: agenthooks.ProviderClaudeCode, Kind: agenthooks.KindPromptSubmitted, Session: agenthooks.SessionInfo{ID: "session"}}
	if u := e.StateUpdate(event, nil, state.Session{}); u.ProviderResolved || calls != 0 {
		t.Fatal(u, calls)
	}
	e.ConfigureProvider(true)
	if u := e.StateUpdate(event, nil, state.Session{}); u.ModelProvider != "bedrock" || !u.ProviderResolved {
		t.Fatal(u)
	}
	prior := state.Session{ModelProvider: "bedrock", ProviderResolved: true, ProviderSource: "claude-environment"}
	env = map[string]string{"CLAUDE_CODE_USE_VERTEX": "1"}
	if u := e.StateUpdate(event, nil, prior); u.ModelProvider != "vertex" {
		t.Fatal(u)
	}
	env["ANTHROPIC_BASE_URL"] = "private"
	if u := e.StateUpdate(event, nil, prior); u.ModelProvider != "" || !u.ProviderResolved {
		t.Fatal("stale provider", u)
	}
	e.ProviderOffline = true
	before := calls
	if u := e.StateUpdate(event, nil, prior); u.ModelProvider != "" || u.ProviderSource != "offline-unavailable" || calls != before {
		t.Fatal(u, calls)
	}
	id := "fact"
	e.OfflineProvider = &id
	if u := e.StateUpdate(event, nil, prior); u.ModelProvider != "fact" || u.ProviderSource != "provided" {
		t.Fatal(u)
	}
}
