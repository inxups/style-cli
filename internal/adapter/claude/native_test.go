package claude

import (
	"runtime"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

func TestNativeHookResolvesEffectiveTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		if _, err := DefaultFacts().NativeHook(spec.NativeMapping{Event: "Stop", Command: "echo native"}); err == nil {
			t.Fatal("Windows accepted POSIX author native command")
		}
		return
	}
	facts := DefaultFacts()
	for _, tc := range []struct {
		rule spec.NativeMapping
		want plan.NativeEntry
	}{
		{spec.NativeMapping{Event: "Stop"}, plan.NativeEntry{Event: "Stop", Timeout: 60}},
		{spec.NativeMapping{Event: "PreToolUse", Matcher: "^Bash$", Timeout: 10}, plan.NativeEntry{Event: "PreToolUse", Matcher: "^Bash$", Timeout: 10}},
		{spec.NativeMapping{Event: "SessionEnd", Timeout: 30}, plan.NativeEntry{Event: "SessionEnd", Timeout: 30}},
		{spec.NativeMapping{Event: "SessionEnd"}, plan.NativeEntry{Event: "SessionEnd", Timeout: 60}},
	} {
		got, err := facts.NativeHook(tc.rule)
		if err != nil || got != tc.want {
			t.Errorf("NativeHook(%#v) = %#v, %v; want %#v", tc.rule, got, err, tc.want)
		}
	}
	if _, err := facts.NativeHook(spec.NativeMapping{Event: "NoSuchEvent"}); err == nil {
		t.Error("unknown native event accepted")
	}
}
