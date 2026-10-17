package when

import (
	"strings"
	"testing"
)

func TestM3A11ProviderExactIDsMissingAndTrace(t *testing.T) {
	rule, err := Normalize(map[string]any{"provider": []any{"openai", "custom.ID"}, "model": "*"}, "tool.pre")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   string
		want bool
	}{{"openai", true}, {"custom.ID", true}, {"OpenAI", false}, {"codex", false}, {"anthropic", false}, {"", false}} {
		t.Run(tc.id, func(t *testing.T) {
			m, _ := Compile(rule)
			got, checks := m.MatchWithTrace(Input{Provider: tc.id, ProviderSource: "codex-launch", Model: "aliased"})
			if got != tc.want || len(checks) == 0 || checks[0].Field != "provider" {
				t.Fatal(got, checks)
			}
			if tc.id == "" && checks[0].Result != "missing-data" {
				t.Fatal(checks)
			}
			if tc.id != "" && checks[0].Source != "codex-launch" {
				t.Fatal(checks)
			}
		})
	}
	for _, id := range []string{"", "https://private.invalid", "a b", strings.Repeat("a", 65), "x\nsecret"} {
		if _, err := Normalize(map[string]any{"provider": id}, "tool.pre"); err == nil {
			t.Fatal("accepted invalid provider")
		}
	}
	if !ValidProviderID(strings.Repeat("A", 64)) || !ValidProviderID("Azure-1.local") {
		t.Fatal("valid bounded ID rejected")
	}
}
