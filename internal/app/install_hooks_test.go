package app

import (
	"slices"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestClaudeToolPostHooksAlsoSubscribeFailures(t *testing.T) {
	index := compile.Index{Harnesses: map[string]map[string][]compile.Entry{"claude-code": {
		plan.ToolPost: {{Rule: "pkg/shell", Target: "tool", Match: &compile.Match{Canonical: []string{"shell"}}}},
	}}}
	hooks := hooksForProvider(adapter.ProviderID("claude-code"), index)
	points := map[string][]string{}
	for _, hook := range hooks {
		points[hook.NativeEvent+" "+hook.Matcher] = hook.Points
	}
	if !slices.Equal(points["PostToolUse ^(Bash)$"], []string{plan.ToolPost}) || !slices.Equal(points["PostToolUseFailure ^(Bash)$"], []string{plan.ToolPost}) {
		t.Fatalf("hooks = %#v", hooks)
	}

	// A package with its own tool.error rule shares the failure hook.
	index.Harnesses["claude-code"][plan.ToolError] = []compile.Entry{{Rule: "pkg/error", Target: "event"}}
	hooks = hooksForProvider(adapter.ProviderID("claude-code"), index)
	failures := 0
	for _, hook := range hooks {
		if hook.NativeEvent != "PostToolUseFailure" {
			continue
		}
		failures++
		switch hook.Matcher {
		case "":
			if !slices.Equal(hook.Points, []string{plan.ToolError}) {
				t.Fatalf("catch-all failure hook = %#v", hook)
			}
		case "^(Bash)$":
			if !slices.Equal(hook.Points, []string{plan.ToolPost}) {
				t.Fatalf("Bash failure hook = %#v", hook)
			}
		default:
			t.Fatalf("unexpected failure hook = %#v", hook)
		}
	}
	if failures != 2 {
		t.Fatalf("hooks = %#v", hooks)
	}
}
