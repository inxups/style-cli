package when

import (
	"encoding/json"
	"testing"
)

func TestNormalizedJSONPreservesMatching(t *testing.T) {
	falseValue := false
	input := Input{
		Model: "gpt-5", ThinkingEffort: "high", ThinkingEffortApprox: &falseValue,
		Harness: map[string]string{"permission_mode": "default"},
	}
	cases := []struct {
		name  string
		value map[string]any
		want  bool
	}{
		{"omitted selectors", nil, true},
		{"empty model", map[string]any{"model": []any{}}, false},
		{"matching model", map[string]any{"model": "GPT-*"}, true},
		{"empty provider", map[string]any{"provider": []any{}}, false},
		{"provider without a data source", map[string]any{"provider": "openai"}, false},
		{"empty agent", map[string]any{"agent": []any{}}, false},
		{"empty harness selector", map[string]any{"harness": map[string]any{"permission_mode": []any{}}}, false},
		{"effort comparison", map[string]any{"thinking_effort": ">=high"}, true},
		{"exact approximation", map[string]any{"thinking_effort_approx": false}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := Normalize(tc.value, "tool.pre")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(rule)
			if err != nil {
				t.Fatal(err)
			}
			var restored Rule
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if before, after := Match(rule, input), Match(restored, input); before != tc.want || after != tc.want {
				t.Fatalf("Match before/after JSON = %v/%v, want %v; JSON: %s", before, after, tc.want, encoded)
			}
		})
	}

	childRule, err := Normalize(nil, "subagent.start")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(childRule)
	if err != nil {
		t.Fatal(err)
	}
	var restored Rule
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !Match(restored, Input{AgentID: "child-1", AgentType: "reviewer"}) {
		t.Fatalf("subagent.start default scope lost in JSON: %s", encoded)
	}
}

func TestJSONPreservesInvalidEmptyEffort(t *testing.T) {
	rule := Rule{ThinkingEffort: []string{}}
	encoded, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}
	var restored Rule
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(restored); err == nil {
		t.Fatalf("invalid empty effort became an unconditional rule after JSON: %s", encoded)
	}
}
