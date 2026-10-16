package claude

import "regexp"

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
