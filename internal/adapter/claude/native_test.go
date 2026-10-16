package claude

import (
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

func TestNativeHookResolvesEffectiveTimeout(t *testing.T) {
	facts := DefaultFacts()
	for _, tc := range []struct {
		rule spec.NativeRule
		want plan.NativeEntry
	}{
		{spec.NativeRule{Event: "Stop"}, plan.NativeEntry{Event: "Stop", Timeout: 60}},
		{spec.NativeRule{Event: "PreToolUse", Matcher: "^Bash$", Timeout: 10}, plan.NativeEntry{Event: "PreToolUse", Matcher: "^Bash$", Timeout: 10}},
		{spec.NativeRule{Event: "SessionEnd", Timeout: 30}, plan.NativeEntry{Event: "SessionEnd", Timeout: 30}},
		{spec.NativeRule{Event: "SessionEnd"}, plan.NativeEntry{Event: "SessionEnd", Timeout: 60}},
	} {
		got, err := facts.NativeHook(tc.rule)
		if err != nil || got != tc.want {
			t.Errorf("NativeHook(%#v) = %#v, %v; want %#v", tc.rule, got, err, tc.want)
		}
	}
	if _, err := facts.NativeHook(spec.NativeRule{Event: "NoSuchEvent"}); err == nil {
		t.Error("unknown native event accepted")
	}
}
