package app

import (
	"reflect"
	"testing"

	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

// TestNativeCatalogComesFromRegisteredAdapters checks that validate learns
// harness keys and native events from the adapter registry (rules §3.10),
// with the event and matcher sets of CLI design §9.3 and §10.2.
func TestNativeCatalogComesFromRegisteredAdapters(t *testing.T) {
	service, err := New(codex.Definition(), claude.Definition())
	if err != nil {
		t.Fatal(err)
	}
	if got := service.catalog.Native.Names(); !reflect.DeepEqual(got, []string{"claude-code", "codex"}) {
		t.Fatalf("catalog harnesses = %v", got)
	}
	cases := []struct {
		harness, event string
		known, matcher bool
	}{
		{"codex", "Stop", true, false},
		{"codex", "UserPromptSubmit", true, false},
		{"codex", "PreToolUse", true, true},
		{"codex", "PermissionRequest", true, true},
		{"codex", "SubagentStop", true, false},
		{"codex", "UserPromptExpansion", false, false},
		{"codex", "PostToolUseFailure", false, false},
		{"claude-code", "UserPromptExpansion", true, false},
		{"claude-code", "PostToolUseFailure", true, true},
		{"claude-code", "FileChanged", true, false},
		{"claude-code", "PostModelSwitch", false, false},
	}
	for _, tc := range cases {
		matcher, known := service.catalog.Native[tc.harness].NativeEvent(tc.event)
		if known != tc.known || matcher != tc.matcher {
			t.Errorf("%s %s = (matcher %v, known %v), want (%v, %v)", tc.harness, tc.event, matcher, known, tc.matcher, tc.known)
		}
	}
	if service.catalog.Native["codex"].ValidMatcher("*") || !service.catalog.Native["claude-code"].ValidMatcher("*") {
		t.Error("only Claude Code accepts the * matcher")
	}
	if service.catalog.Native["codex"].ValidMatcher("(") || service.catalog.Native["claude-code"].ValidMatcher("(") {
		t.Error("invalid regex accepted")
	}
}
