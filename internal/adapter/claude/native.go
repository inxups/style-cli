package claude

import (
	"fmt"
	"regexp"
	"runtime"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
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

// Native retains the author's command and uses the same effective fields as
// planning. The shared renderer supplies the ownership prefix.
func (f Facts) Native(rule spec.NativeMapping) (adapter.HookSpec, error) {
	entry, err := f.NativeHook(rule)
	if err != nil {
		return adapter.HookSpec{}, err
	}
	return adapter.HookSpec{NativeEvent: entry.Event, Matcher: entry.Matcher,
		Extra:   map[string]any{"timeout_seconds": entry.Timeout},
		Command: rule.Command, NativeTag: rule.Package + "/" + rule.HookID}, nil
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
func (f Facts) NativeHook(rule spec.NativeMapping) (plan.NativeEntry, error) {
	if runtime.GOOS == "windows" {
		return plan.NativeEntry{}, fmt.Errorf("native author commands require POSIX; unavailable on Windows")
	}
	if _, ok := nativeEvents[rule.Event]; !ok {
		return plan.NativeEntry{}, fmt.Errorf("Claude Code has no native event %q", rule.Event)
	}
	timeout := rule.Timeout
	if timeout == 0 {
		timeout = defaultNativeTimeout
	}
	return plan.NativeEntry{Event: rule.Event, Matcher: rule.Matcher, Timeout: timeout}, nil
}
