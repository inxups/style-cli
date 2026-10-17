package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
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

// Status checks the runner surface while retaining native commands currently
// on disk. Doctor separately checks those against the approved package bytes.
func (r *route) installedNative() ([]adapter.HookSpec, error) {
	path := filepath.Join(r.dir, "hooks.json")
	if r.name == PluginRouteName {
		path = filepath.Join(r.dir, "hooks", "hooks.json")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var document struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	var hooks []adapter.HookSpec
	for event, groups := range document.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				const prefix = ": --owner=" + Owner + " --native="
				if !strings.HasPrefix(hook.Command, prefix) {
					continue
				}
				tag, command, ok := strings.Cut(strings.TrimPrefix(hook.Command, prefix), "; ")
				if !ok {
					continue
				}
				hooks = append(hooks, adapter.HookSpec{NativeEvent: event, Matcher: group.Matcher,
					Command: command, NativeTag: tag, Extra: map[string]any{"timeout_seconds": hook.Timeout}})
			}
		}
	}
	return hooks, nil
}

// Native retains the author's command and uses the same effective fields as
// planning. The shared renderer supplies the ownership prefix.
func (f Facts) Native(rule spec.NativeRule) (adapter.HookSpec, error) {
	entry, err := f.NativeHook(rule)
	if err != nil {
		return adapter.HookSpec{}, err
	}
	return adapter.HookSpec{NativeEvent: entry.Event, Matcher: entry.Matcher,
		Extra:   map[string]any{"timeout_seconds": entry.Timeout},
		Command: rule.Command, NativeTag: rule.Package + "/" + rule.RuleID}, nil
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
	if runtime.GOOS == "windows" {
		return plan.NativeEntry{}, fmt.Errorf("native author commands require POSIX; unavailable on Windows")
	}
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
