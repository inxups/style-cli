package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks"
)

// RuntimeStatus describes only this owner's runner subscriptions, not native
// author commands. Read is supplied by the consumer's bounded file capture.
type RuntimeStatus struct{ Events []string }

func InspectRuntime(m Manifest, t Target, read func(string) ([]byte, error)) (RuntimeStatus, error) {
	result := RuntimeStatus{Events: []string{}}
	rendered, err := Render(m, t)
	if err != nil {
		return result, err
	}
	name := "hooks.json"
	if t.Provider == agenthooks.ProviderClaudeCode {
		name = "settings.json"
	}
	if t.Scope == ScopePlugin {
		name = "hooks/hooks.json"
	}
	raw, err := read(filepath.Join(t.Dir, filepath.FromSlash(name)))
	if err != nil {
		return result, err
	}
	var actual struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &actual); err != nil {
		return result, errors.New("invalid runtime hook configuration")
	}
	var trust struct {
		Hooks struct {
			State map[string]struct {
				Hash string `toml:"trusted_hash"`
			} `toml:"state"`
		} `toml:"hooks"`
		Plugins map[string]struct {
			Enabled bool `toml:"enabled"`
		} `toml:"plugins"`
	}
	if t.Provider == agenthooks.ProviderCodex {
		configRoot := t.Dir
		if t.Scope == ScopePlugin {
			configRoot = t.ConfigDir
		}
		data, err := read(filepath.Join(configRoot, "config.toml"))
		if err != nil {
			return result, err
		}
		if toml.Unmarshal(data, &trust) != nil {
			return result, errors.New("invalid runtime trust configuration")
		}
		if t.Scope == ScopePlugin && !trust.Plugins[m.Identity.PluginID].Enabled {
			return result, errors.New("runtime plugin is not enabled")
		}
	}
	expectedRaw, err := fs.ReadFile(rendered, name)
	if err != nil {
		return result, err
	}
	var expected struct {
		Hooks map[string][]claudeMatcherEntry `json:"hooks"`
	}
	if err := json.Unmarshal(expectedRaw, &expected); err != nil {
		return result, err
	}
	events := map[string]bool{}
	for event, groups := range actual.Hooks {
		for groupIndex, groupRaw := range groups {
			var group struct {
				Matcher string            `json:"matcher"`
				Hooks   []json.RawMessage `json:"hooks"`
			}
			if json.Unmarshal(groupRaw, &group) != nil {
				return result, errors.New("invalid runtime hook group")
			}
			for handlerIndex, hookRaw := range group.Hooks {
				var hook claudeHookCmd
				if json.Unmarshal(hookRaw, &hook) != nil {
					return result, errors.New("invalid runtime hook handler")
				}
				if !handlerHasOwner(hook.Command, hook.Args, manifestOwner(m)) {
					continue
				}
				// The no-op ownership prefix belongs to a native author command.
				if strings.HasPrefix(hook.Command, ": --owner="+manifestOwner(m)+" ") {
					continue
				}
				decoder := json.NewDecoder(bytes.NewReader(hookRaw))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&hook) != nil || hook.Type != "command" || len(group.Hooks) != 1 {
					return result, errors.New("runtime hook definition changed")
				}
				matches := false
				candidates := expected.Hooks[event]
				if t.Provider == agenthooks.ProviderClaudeCode {
					single := m
					// A rendered default timeout does not imply an explicit runner
					// timeout argument. Retain the actual duration (including fractions)
					// and let the renderer verify both representations together.
					var timeout time.Duration
					seenTimeout := false
					for _, arg := range hook.Args {
						if value, ok := strings.CutPrefix(arg, "--timeout="); ok {
							var parseErr error
							timeout, parseErr = time.ParseDuration(value)
							if parseErr != nil || timeout <= 0 || seenTimeout {
								return result, errors.New("runtime hook timeout changed")
							}
							seenTimeout = true
						}
					}
					single.Hooks = []HookSpec{{NativeEvent: event, NativeMatcher: group.Matcher, Blocking: !hook.Async, Timeout: timeout}}
					for kind, native := range kindToClaude {
						if native == event {
							single.Hooks[0].Kind = kind
							break
						}
					}
					if hook.OnFailure == "block" {
						if event != "PreToolUse" {
							return result, errors.New("runtime hook policy changed")
						}
						single.Fail = agenthooks.FailClosed
						single.Hooks[0].Kind = agenthooks.KindToolPre
					} else if hook.OnFailure != "" {
						return result, errors.New("runtime hook policy changed")
					}
					singleRendered, renderErr := Render(single, t)
					if renderErr != nil {
						return result, renderErr
					}
					data, _ := fs.ReadFile(singleRendered, name)
					var one struct {
						Hooks map[string][]claudeMatcherEntry `json:"hooks"`
					}
					if json.Unmarshal(data, &one) != nil {
						return result, errors.New("invalid runtime rendering")
					}
					candidates = one.Hooks[event]
				}
				for _, candidate := range candidates {
					if candidate.Matcher == group.Matcher && len(candidate.Hooks) == 1 && reflect.DeepEqual(candidate.Hooks[0], hook) {
						matches = true
					}
				}
				if !matches {
					return result, errors.New("runtime command or subscription changed")
				}
				if t.Provider == agenthooks.ProviderCodex {
					entry, ok := trust.Hooks.State[codexTrustKey(m, t, event, groupIndex)]
					if !ok || handlerIndex != 0 || entry.Hash != DefinitionHash(event, group.Matcher, hook.Command, hook.Timeout) {
						return result, errors.New("runtime trust binding changed")
					}
				}
				events[event] = true
			}
		}
	}
	if t.Provider == agenthooks.ProviderCodex && len(events) > 0 {
		for event, groups := range expected.Hooks {
			for _, group := range groups {
				for _, hook := range group.Hooks {
					if !strings.HasPrefix(hook.Command, ": --owner="+manifestOwner(m)+" ") && !events[event] {
						return result, fmt.Errorf("runtime subscription missing: %s", event)
					}
				}
			}
		}
	}
	if t.Scope == ScopePlugin {
		for _, path := range []string{"bin/style", ".codex-plugin/plugin.json"} {
			expected, err := fs.ReadFile(rendered, path)
			if err != nil {
				return result, err
			}
			actual, err := read(filepath.Join(t.Dir, filepath.FromSlash(path)))
			if err != nil {
				return result, err
			}
			if !bytes.Equal(actual, expected) {
				return result, errors.New("runtime plugin artifact changed")
			}
		}
	}
	for event := range events {
		result.Events = append(result.Events, event)
	}
	sort.Strings(result.Events)
	return result, nil
}
