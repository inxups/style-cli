package codex

import "regexp"

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
