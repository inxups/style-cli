package claude

import (
	"fmt"
	"regexp"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

// nativeEvents lists the Claude Code hook events a target: native rule may
// write (CLI design §10.2), matching agenthooks' kindToClaude. The value
// reports whether the event accepts a matcher; M2 opens tool events only.
var nativeEvents = map[string]bool{
	"SessionStart":        false,
	"SessionEnd":          false,
	"UserPromptSubmit":    false,
	"UserPromptExpansion": false,
	"PreToolUse":          true,
	"PostToolUse":         true,
	"PostToolUseFailure":  true,
	"PermissionRequest":   true,
	"Stop":                false,
	"SubagentStart":       false,
	"SubagentStop":        false,
	"PreCompact":          false,
	"PostCompact":         false,
	"Notification":        false,
	"FileChanged":         false,
}

// NativeSurface describes Claude Code native hooks for core/spec validation.
type NativeSurface struct{}

func (NativeSurface) NativeEvent(event string) (matcher bool, ok bool) {
	matcher, ok = nativeEvents[event]
	return matcher, ok
}

// ValidMatcher accepts "*" (every tool) or a pattern that compiles as a
// regular expression.
func (NativeSurface) ValidMatcher(matcher string) bool {
	if matcher == "*" {
		return true
	}
	_, err := regexp.Compile(matcher)
	return err == nil
}

// defaultNativeTimeout is used when a native mapping omits timeout.
const defaultNativeTimeout = 60

// NativeHook resolves one native mapping for planning: the effective timeout
// is the one the route will write.
func (f Facts) NativeHook(rule spec.NativeRule) (plan.NativeEntry, error) {
	if _, ok := nativeEvents[rule.Event]; !ok {
		return plan.NativeEntry{}, fmt.Errorf("Claude Code has no native event %q", rule.Event)
	}
	timeout := rule.Timeout
	if timeout == 0 {
		timeout = defaultNativeTimeout
	}
	return plan.NativeEntry{Event: rule.Event, Matcher: rule.Matcher, Timeout: timeout}, nil
}
