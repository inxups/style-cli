package conformance_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/speakeasy-api/agenthooks"
)

// Check stored goldens as well as freshly emitted bytes: an intentional golden
// update must never approve a field outside STYLE's output contract.
func TestEveryWireGoldenObeysOutputWhitelist(t *testing.T) {
	for _, suite := range suites() {
		paths, err := filepath.Glob(filepath.Join(corpus, string(suite.id), "*.stdout.golden"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("goldens: %v", err)
		}
		for _, path := range paths {
			t.Run(string(suite.id)+"/"+filepath.Base(path), func(t *testing.T) { checkWhitelist(t, suite.id, read(t, path)) })
		}
	}
}

func checkWhitelist(t *testing.T, provider agenthooks.Provider, raw []byte) {
	t.Helper()
	if len(raw) == 0 {
		if provider != agenthooks.ProviderCodex {
			t.Fatal("Claude no-op must be {}")
		}
		return
	}
	var output map[string]json.RawMessage
	if err := json.Unmarshal(raw, &output); err != nil || output == nil {
		t.Fatalf("invalid output %q: %v", raw, err)
	}
	for key, value := range output {
		switch key {
		case "decision":
			if string(value) != `"block"` {
				t.Fatalf("unexpected decision %s", value)
			}
		case "reason", "systemMessage":
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				t.Fatalf("%s must be a string", key)
			}
		case "hookSpecificOutput":
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(value, &nested); err != nil {
				t.Fatal(err)
			}
			for field, data := range nested {
				switch field {
				case "hookEventName", "additionalContext", "permissionDecisionReason":
					var text string
					if err := json.Unmarshal(data, &text); err != nil {
						t.Fatalf("%s must be a string", field)
					}
				case "permissionDecision":
					if string(data) != `"deny"` {
						t.Fatalf("unexpected permission %s", data)
					}
				case "suppressOriginalPrompt":
					if provider != agenthooks.ProviderClaudeCode || string(data) != "true" || string(nested["hookEventName"]) != `"UserPromptSubmit"` {
						t.Fatal("invalid prompt suppression")
					}
				default:
					t.Fatalf("forbidden nested field %s", field)
				}
			}
		default:
			t.Fatalf("forbidden top-level field %s", key)
		}
	}
	if provider == agenthooks.ProviderCodex && strings.TrimSpace(string(raw)) == "{}" {
		t.Fatal("Codex no-op must be empty")
	}
}
