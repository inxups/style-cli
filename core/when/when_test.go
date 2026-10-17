package when

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExpandEffort(t *testing.T) {
	cases := []struct {
		input any
		want  []string
	}{
		{input: ">=HIGH", want: []string{"high", "xhigh", "max"}},
		{input: "<=low", want: []string{"none", "minimal", "low"}},
		{input: []any{"minimal", "OPEN"}, want: []string{"minimal", "open"}},
		{input: "open", want: []string{"open"}},
	}
	for _, tc := range cases {
		got, err := ExpandEffort(tc.input)
		if err != nil {
			t.Errorf("ExpandEffort(%#v): %v", tc.input, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ExpandEffort(%#v) = %#v, want %#v", tc.input, got, tc.want)
		}
	}
	for _, input := range []any{">=open", "<=open", "persistent", []any{}} {
		if _, err := ExpandEffort(input); err == nil {
			t.Errorf("ExpandEffort(%#v) unexpectedly succeeded", input)
		}
	}
}

func TestNormalizeDefaultsAndMatchingAndOr(t *testing.T) {
	rule, err := Normalize(map[string]any{
		"model":           []any{"GPT-5*", "**/claude-*"},
		"thinking_effort": ">=high",
		"harness": map[string]any{
			"name":            []any{"codex", "claude-code"},
			"permission_mode": "default",
		},
		"conditions": []any{
			map[string]any{"field": "tool.input.command", "op": "contains", "pattern": "git diff"},
			map[string]any{"field": "tool.name", "op": "equals", "pattern": "Bash"},
		},
	}, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rule.Agent, []string{"main"}) {
		t.Fatalf("default agent = %#v, want main", rule.Agent)
	}
	if got := rule.Model[0]; got != "gpt-5*" {
		t.Fatalf("normalized model = %q, want lowercase", got)
	}
	input := Input{
		Model:          "anthropic/claude-opus-5-5",
		ThinkingEffort: "XHIGH",
		Harness:        map[string]string{"name": "codex", "permission_mode": "default"},
		Fields: FieldMap{
			"tool.input.command": "git diff --stat",
			"tool.name":          "Bash",
		},
	}
	if !Match(rule, input) {
		t.Fatal("expected OR within model patterns and AND across when fields/conditions")
	}
	input.Fields = FieldMap{"tool.input.command": "git diff"}
	if Match(rule, input) {
		t.Fatal("all conditions must be ANDed")
	}
	input.Fields = FieldMap{"tool.input.command": "git diff", "tool.name": "Bash"}
	input.Harness["permission_mode"] = "plan"
	if Match(rule, input) {
		t.Fatal("all configured harness selectors must match")
	}
}

func TestNormalizePreservesEmptyModelAlternativesAsNoMatch(t *testing.T) {
	rule, err := Normalize(map[string]any{"model": []any{}}, "prompt.submitted")
	if err != nil {
		t.Fatal(err)
	}
	if Match(rule, Input{Model: "gpt-5"}) {
		t.Fatal("an explicit empty OR list must not behave like an omitted selector")
	}
}

func TestModelGlobSlashAndDoubleStar(t *testing.T) {
	cases := []struct {
		pattern, model string
		want           bool
	}{
		{"gpt-5*", "GPT-5-CODEX", true},
		{"gpt-*", "openai/gpt-5", false},
		{"**/claude-*", "claude-opus-5-5", true},
		{"**/claude-*", "anthropic/claude-opus-5-5", true},
		{"**/claude-*", "org/team/claude-opus", true},
		{"**/claude-*", "anthropic/other", false},
	}
	for _, tc := range cases {
		got := anyModelMatches([]string{tc.pattern}, tc.model)
		if got != tc.want {
			t.Errorf("model %q with pattern %q = %v, want %v", tc.model, tc.pattern, got, tc.want)
		}
	}
}

func TestEffortAndAgentDefaultsAndMissingData(t *testing.T) {
	mainRule, err := Normalize(map[string]any{"thinking_effort": "<=low"}, "prompt.submitted")
	if err != nil {
		t.Fatal(err)
	}
	if Match(mainRule, Input{ThinkingEffort: ""}) {
		t.Fatal("missing effort must not match")
	}
	if Match(mainRule, Input{ThinkingEffort: "medium"}) {
		t.Fatal("medium is outside <=low")
	}
	if !Match(mainRule, Input{ThinkingEffort: "NONE"}) {
		t.Fatal("effort comparisons are case-insensitive")
	}
	if Match(mainRule, Input{ThinkingEffort: "open"}) {
		t.Fatal("open must not match ordered comparisons")
	}

	mainRule, err = Normalize(nil, "prompt.submitted")
	if err != nil {
		t.Fatal(err)
	}
	if !Match(mainRule, Input{}) {
		t.Fatal("an omitted agent selector defaults to main")
	}
	if Match(mainRule, Input{AgentID: "child-1", AgentType: "reviewer"}) {
		t.Fatal("default main agent selector must exclude subagents")
	}
	childRule, err := Normalize(nil, "subagent.start")
	if err != nil {
		t.Fatal(err)
	}
	if !Match(childRule, Input{AgentID: "child-1", AgentType: "reviewer"}) {
		t.Fatal("subagent.start defaults to any agent")
	}
	anyRule, err := Normalize(map[string]any{"agent": []any{"any", "reviewer"}}, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	if !Match(anyRule, Input{AgentID: "child-1", AgentType: "reviewer"}) || !Match(anyRule, Input{}) {
		t.Fatal("any in an agent list must normalize to an unrestricted agent selector")
	}
}

func TestConditionsOperatorsAndUnavailableNegation(t *testing.T) {
	cases := []struct {
		condition Condition
		value     string
		want      bool
	}{
		{Condition{"prompt", "regex", `git\s+diff`}, "please git diff now", true},
		{Condition{"prompt", "contains", "Git"}, "git", false},
		{Condition{"prompt", "equals", "git diff"}, "git diff", true},
		{Condition{"prompt", "not_contains", "push"}, "git diff", true},
		{Condition{"prompt", "starts_with", "git"}, "git diff", true},
		{Condition{"prompt", "ends_with", "diff"}, "git diff", true},
	}
	for _, tc := range cases {
		rule := Rule{Agent: []string{"*"}, Conditions: []Condition{tc.condition}}
		if got := Match(rule, Input{Fields: FieldMap{"prompt": tc.value}}); got != tc.want {
			t.Errorf("condition %#v on %q = %v, want %v", tc.condition, tc.value, got, tc.want)
		}
	}
	rule := Rule{Agent: []string{"*"}, Conditions: []Condition{{Field: "tool.added", Op: "not_contains", Pattern: "TODO"}}}
	if Match(rule, Input{Fields: FieldMap{}}) {
		t.Fatal("not_contains on an unavailable field must not match")
	}
	if !Match(rule, Input{Fields: FieldMap{"tool.added": "new implementation"}}) {
		t.Fatal("not_contains should match when the field exists without the pattern")
	}
}

func TestCompileRejectsInvalidPatternsAndOperators(t *testing.T) {
	for _, rule := range []Rule{
		{Agent: []string{"*"}, Model: []string{"gpt-["}},
		{Agent: []string{"*"}, Conditions: []Condition{{Field: "prompt", Op: "regex", Pattern: "["}}},
		{Agent: []string{"*"}, Conditions: []Condition{{Field: "prompt", Op: "unknown", Pattern: "x"}}},
	} {
		if _, err := Compile(rule); err == nil {
			t.Errorf("Compile(%#v) unexpectedly succeeded", rule)
		}
	}
}

func TestApproximationAndProviderSemantics(t *testing.T) {
	falseValue := false
	rule, err := Normalize(map[string]any{"thinking_effort_approx": false}, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	if Match(rule, Input{}) || !Match(rule, Input{ThinkingEffortApprox: &falseValue}) {
		t.Fatal("approximation matching must require present, equal data")
	}
	providerRule, err := Normalize(map[string]any{"provider": "openai"}, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	if Match(providerRule, Input{}) {
		t.Fatal("provider conditions require a present service identifier")
	}
	encoded, err := json.Marshal(providerRule)
	if err != nil {
		t.Fatal(err)
	}
	var restored Rule
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Provider == nil || len(restored.Provider) != 1 || Match(restored, Input{}) {
		t.Fatalf("provider condition did not survive normalized JSON round-trip: %s / %#v", encoded, restored)
	}
}
