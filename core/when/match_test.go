package when

import "testing"

func TestMainAgentRequiresAbsentID(t *testing.T) {
	rule, err := Normalize(nil, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	if !Match(rule, Input{AgentType: "reviewer"}) {
		t.Fatal("a main agent with a custom type must still match main")
	}
	if Match(rule, Input{AgentID: "child-1", AgentType: "main"}) {
		t.Fatal("a subagent ID must exclude the main selector regardless of its type")
	}
	reviewerRule, err := Normalize(map[string]any{"agent": "reviewer"}, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	if !Match(reviewerRule, Input{AgentID: "child-1", AgentType: "reviewer"}) || Match(reviewerRule, Input{AgentType: "reviewer"}) {
		t.Fatal("a named subagent selector must require both an ID and the matching type")
	}
}

func TestFieldReaderDistinguishesEmptyAndUnavailable(t *testing.T) {
	rule := Rule{Conditions: []Condition{{Field: "prompt", Op: "equals", Pattern: ""}}}
	reader := FieldFunc(func(field string) (string, bool) {
		return "", field == "prompt"
	})
	if !Match(rule, Input{Fields: reader}) {
		t.Fatal("an available empty field must match an empty pattern")
	}
	for _, fields := range []FieldReader{nil, FieldMap{}, FieldFunc(nil)} {
		if Match(rule, Input{Fields: fields}) {
			t.Fatalf("an unavailable field must not match: %#v", fields)
		}
	}
}

func TestCompiledMatcherOwnsRuleAndCanBeReused(t *testing.T) {
	falseValue := false
	rule := Rule{
		Model: []string{"gpt-*"}, ThinkingEffort: []string{"high"}, ThinkingEffortApprox: &falseValue,
		Harness: map[string][]string{"permission_mode": {"default"}}, Agent: []string{"main"},
		Conditions: []Condition{{Field: "prompt", Op: "regex", Pattern: "review"}},
	}
	matcher, err := Compile(rule)
	if err != nil {
		t.Fatal(err)
	}
	rule.Model[0], rule.ThinkingEffort[0], rule.Agent[0] = "other", "low", "reviewer"
	rule.Harness["permission_mode"][0], rule.Conditions[0].Pattern = "plan", "other"
	falseValue = true
	inputApprox := false
	input := Input{
		Model: "gpt-5", ThinkingEffort: "high", ThinkingEffortApprox: &inputApprox,
		Harness: map[string]string{"permission_mode": "default"}, Fields: FieldMap{"prompt": "please review"},
	}
	if !matcher.Match(input) {
		t.Fatal("mutating the source rule changed the compiled matcher")
	}
	input.Fields = FieldMap{"prompt": "skip"}
	if matcher.Match(input) {
		t.Fatal("reusing the matcher must evaluate the new input")
	}
}
