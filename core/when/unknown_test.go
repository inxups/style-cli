package when

import "testing"

func TestM3A12UnknownSelectorsAndCanonicalPresence(t *testing.T) {
	for _, field := range []string{"model", "provider", "thinking_effort", "harness.permission_mode"} {
		for _, mode := range []string{"missing", "empty", "known", "other", "literal-unknown", "mixed"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				selector := any("unknown")
				if mode == "mixed" {
					selector = []any{"known", "unknown"}
				}
				fields := map[string]any{field: selector}
				if field == "harness.permission_mode" {
					fields = map[string]any{"harness": map[string]any{"permission_mode": selector}}
				}
				if field == "thinking_effort" && mode == "mixed" {
					fields[field] = []any{"high", "unknown"}
				}
				rule, err := Normalize(fields, "tool.pre")
				if err != nil {
					t.Fatal(err)
				}
				input := Input{}
				value := ""
				if mode == "known" || mode == "mixed" {
					value = "known"
					if field == "thinking_effort" {
						value = "high"
					}
				}
				if mode == "other" {
					value = "other"
				}
				if mode == "literal-unknown" {
					value = "unknown"
				}
				switch field {
				case "model":
					input.Model = value
				case "provider":
					input.Provider = value
				case "thinking_effort":
					input.ThinkingEffort = value
				case "harness.permission_mode":
					if mode != "missing" {
						input.Harness = map[string]string{"permission_mode": value}
					}
				}
				matcher, _ := Compile(rule)
				got, trace := matcher.MatchWithTrace(input)
				want := value == "" || mode == "mixed"
				if got != want || len(trace) == 0 {
					t.Fatal(got, want, trace)
				}
				if value == "" && (trace[0].Available || trace[0].Result != "pass" || trace[0].Source != "unavailable") {
					t.Fatal(trace)
				}
			})
		}
	}
	rule, err := Normalize(map[string]any{"model": "unknown", "provider": "unknown", "thinking_effort": "unknown", "harness": map[string]any{"permission_mode": "unknown"}}, "tool.pre")
	if err != nil || !Match(rule, Input{}) {
		t.Fatal(rule, err)
	}
	if Match(rule, Input{Model: "known"}) {
		t.Fatal("cross-field AND lost")
	}
	if !AllowsUnknown("model", []string{"UNKNOWN"}) || !AllowsUnknown("thinking_effort", " UNKNOWN ") || AllowsUnknown("provider", "UNKNOWN") || AllowsUnknown("agent", "unknown") {
		t.Fatal("field normalization")
	}
}
func TestM3A12UnknownComparisonsGlobAndUnchangedLiterals(t *testing.T) {
	for _, raw := range []any{">=unknown", "<=unknown", []any{"high", ">=unknown"}} {
		if _, err := ExpandEffort(raw); err == nil {
			t.Fatal("ordered unknown accepted")
		}
	}
	for _, raw := range []any{">=high", "<=max"} {
		expanded, err := ExpandEffort(raw)
		if err != nil || AllowsUnknown("thinking_effort", expanded) || Match(Rule{ThinkingEffort: expanded}, Input{}) {
			t.Fatal(raw, expanded, err)
		}
	}
	for _, pattern := range []string{"*", "**/*", "gpt-*"} {
		rule, err := Normalize(map[string]any{"model": pattern}, "tool.pre")
		if err != nil || Match(rule, Input{}) {
			t.Fatal(pattern, err)
		}
	}
	rule, _ := Normalize(map[string]any{"model": []any{"unknown", "**/claude-*"}}, "tool.pre")
	if !Match(rule, Input{}) || !Match(rule, Input{Model: "host/Claude-a"}) || Match(rule, Input{Model: "gpt-a"}) {
		t.Fatal(rule)
	}
	literal, _ := Normalize(map[string]any{"agent": "unknown"}, "tool.pre")
	if Match(literal, Input{AgentID: "child"}) || !Match(literal, Input{AgentID: "child", AgentType: "unknown"}) {
		t.Fatal("agent literal changed")
	}
	for _, op := range []string{"contains", "not_contains"} {
		r := Rule{Conditions: []Condition{{Field: "prompt", Op: op, Pattern: "unknown"}}}
		if Match(r, Input{}) {
			t.Fatal("missing condition matched", op)
		}
	}
	if !Match(Rule{Conditions: []Condition{{Field: "prompt", Op: "equals", Pattern: "unknown"}}}, Input{Fields: FieldMap{"prompt": "unknown"}}) {
		t.Fatal("condition literal changed")
	}
	if !Match(Rule{Conditions: []Condition{{Field: "prompt", Op: "not_contains", Pattern: "unknown"}}}, Input{Fields: FieldMap{"prompt": ""}}) {
		t.Fatal("explicit empty condition changed")
	}
}
