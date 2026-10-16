package codex

import (
	"fmt"
	"regexp"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

// nativeEvents lists the Codex hook events a target: native rule may write
// (CLI design §9.3), matching agenthooks' codexEventLabel. The value reports
// whether the event accepts a matcher; M2 opens matchers on tool events only.
var nativeEvents = map[string]bool{
	"SessionStart":      false,
	"SessionEnd":        false,
	"UserPromptSubmit":  false,
	"PreToolUse":        true,
	"PermissionRequest": true,
	"PostToolUse":       true,
	"PreCompact":        false,
	"PostCompact":       false,
	"Stop":              false,
	"SubagentStart":     false,
	"SubagentStop":      false,
}

// NativeSurface describes Codex native hooks for core/spec validation.
type NativeSurface struct{}

func (NativeSurface) NativeEvent(event string) (matcher bool, ok bool) {
	matcher, ok = nativeEvents[event]
	return matcher, ok
}

// ValidMatcher accepts patterns that compile as regular expressions; Codex
// matches hook matchers as regexes.
func (NativeSurface) ValidMatcher(matcher string) bool {
	_, err := regexp.Compile(matcher)
	return err == nil
}

// defaultNativeTimeout is used when a native mapping omits timeout.
const defaultNativeTimeout = 60

// NativeHook resolves one native mapping for planning: the effective timeout
// is the one the route will write.
func (f Facts) NativeHook(rule spec.NativeRule) (plan.NativeEntry, error) {
	if _, ok := nativeEvents[rule.Event]; !ok {
		return plan.NativeEntry{}, fmt.Errorf("Codex has no native event %q", rule.Event)
	}
	timeout := rule.Timeout
	if timeout == 0 {
		timeout = defaultNativeTimeout
	}
	if rule.Event == "SessionEnd" && timeout > 3 {
		timeout = 3 // agenthooks caps Codex SessionEnd hooks at 3 seconds.
	}
	return plan.NativeEntry{Event: rule.Event, Matcher: rule.Matcher, Timeout: timeout}, nil
}
