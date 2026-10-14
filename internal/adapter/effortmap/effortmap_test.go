package effortmap

import (
	"strings"
	"testing"
)

func TestParseAndResolveProviderTables(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want map[string]string
	}{
		{name: "codex", data: "harness: codex\nexact: [none, minimal, low, medium, high, xhigh, max]\naliases:\n  ultra: max\n", want: map[string]string{
			"none": "none", "minimal": "minimal", "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh", "max": "max", "ultra": "max",
		}},
		{name: "claude", data: "harness: claude-code\nexact: [low, medium, high, xhigh, max]\naliases: {}\n", want: map[string]string{
			"low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh", "max": "max",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table, err := Parse(strings.NewReader(tc.data))
			if err != nil {
				t.Fatal(err)
			}
			for raw, expected := range tc.want {
				value := table.Resolve(raw)
				if value.Effort != expected || value.EffortRaw != raw || value.EffortApprox == nil || *value.EffortApprox {
					t.Errorf("Resolve(%q) = %#v, want exact %q", raw, value, expected)
				}
			}
		})
	}
}

func TestResolvePreservesUnknownRawWithoutClaimingApproximation(t *testing.T) {
	table, err := Parse(strings.NewReader("harness: codex\nexact: [low]\naliases: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"persistent", "custom-value", "  future  ", ""} {
		got := table.Resolve(raw)
		if got.Effort != "" || got.EffortRaw != raw || got.EffortApprox != nil {
			t.Errorf("Resolve(%q) = %#v, want unavailable raw-preserving value", raw, got)
		}
	}
	if got := table.Resolve(" LOW "); got.Effort != "low" || got.EffortApprox == nil || *got.EffortApprox {
		t.Fatalf("case/space normalized exact mapping = %#v", got)
	}
}

func TestParseRejectsInvalidOrAmbiguousTables(t *testing.T) {
	for _, data := range []string{
		"exact: [low]\naliases: {}\n",
		"harness: codex\nexact: [low, LOW]\naliases: {}\n",
		"harness: codex\nexact: [custom]\naliases: {}\n",
		"harness: codex\nexact: [low]\naliases:\n  ultra: custom\n",
		"harness: codex\nexact: [low]\naliases: {}\n---\nharness: other\nexact: [high]\naliases: {}\n",
	} {
		if _, err := Parse(strings.NewReader(data)); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", data)
		}
	}
}
