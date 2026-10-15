package conformance_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/agenthookstest"
)

func TestA11AgentDefaultsAnyAndSubagentStartRepeat(t *testing.T) {
	const manifest = "spec: style/v0.3\nname: scope\nversion: 1.0.0\ndescription: Agent scope\nrules:\n  - id: shell\n    target: tool\n    tool: shell\n    at: before\n    text: Scope context.\n"
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			payload := read(t, filepath.Join(corpus, string(suite.id), "subagent_tool.json"))
			index := buildIndex(t, suite, []byte(manifest))
			result, _ := invoke(t, suite, index, payload, newStore(t))
			agenthookstest.AssertNoOp(t, suite.id, result)
			// A named top-level agent still has no agent_id and is the main agent.
			var namedMain map[string]any
			if err := json.Unmarshal(payload, &namedMain); err != nil {
				t.Fatal(err)
			}
			delete(namedMain, "agent_id")
			namedPayload, err := json.Marshal(namedMain)
			if err != nil {
				t.Fatal(err)
			}
			result, _ = invoke(t, suite, index, namedPayload, newStore(t))
			if !strings.Contains(string(result.Stdout), "Scope context.") {
				t.Fatalf("named main agent did not match: %s", result.Stdout)
			}
			index = buildIndex(t, suite, []byte(manifest+"    when: {agent: any}\n"))
			store := newStore(t)
			result, _ = invoke(t, suite, index, payload, store)
			if !strings.Contains(string(result.Stdout), "Scope context.") {
				t.Fatalf("explicit any did not match: %s", result.Stdout)
			}
			result, _ = invoke(t, suite, index, payload, store)
			agenthookstest.AssertNoOp(t, suite.id, result)
			// Claude SubagentStart is observe-only, as recorded by the plan.
			if suite.id == agenthooks.ProviderCodex {
				index = buildIndex(t, suite, read(t, filepath.Join(corpus, "STYLE.yaml")))
				payload = read(t, filepath.Join(corpus, string(suite.id), "native_SubagentStart.json"))
				store = newStore(t)
				for range 2 {
					result, _ = invoke(t, suite, index, payload, store)
					if !strings.Contains(string(result.Stdout), "Subagent context.") {
						t.Fatalf("subagent.start was filtered or deduplicated: %s", result.Stdout)
					}
				}
			}
		})
	}
}
