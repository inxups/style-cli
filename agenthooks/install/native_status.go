package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks"
)

// NativeStatus compares a tagged command with an approved manifest. Trust is
// checked only for Codex user hooks; plugin/workspace trust is external state.
type NativeStatus struct {
	Tag   string
	State string // ok, missing, modified
	Trust string // match, missing, mismatch, unreadable; empty for external trust
}

// InspectNative reads configuration without modifying it or executing hooks.
func InspectNative(m Manifest, t Target) ([]NativeStatus, error) {
	rendered, err := Render(m, t)
	if err != nil {
		return nil, err
	}
	path := "hooks.json"
	if t.Scope == ScopePlugin {
		path = "hooks/hooks.json"
	}
	if t.Provider == agenthooks.ProviderClaudeCode {
		path = "settings.json"
		if t.Scope == ScopeProject {
			path = ".claude/settings.json"
		}
		if t.Scope == ScopePlugin {
			path = "hooks/hooks.json"
		}
	}
	data, err := fs.ReadFile(rendered, path)
	if err != nil {
		return nil, err
	}
	var expected, actual struct {
		Hooks map[string][]claudeMatcherEntry `json:"hooks"`
	}
	if err := json.Unmarshal(data, &expected); err != nil {
		return nil, err
	}
	data, err = os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(path)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &actual); err != nil {
			return nil, err
		}
	}
	var trust struct {
		Hooks struct {
			State map[string]struct {
				Hash string `toml:"trusted_hash"`
			} `toml:"state"`
		} `toml:"hooks"`
	}
	var trustErr error
	if t.Provider == agenthooks.ProviderCodex && (t.Scope == ScopeUser || t.Scope == ScopeProject) {
		data, trustErr = os.ReadFile(filepath.Join(renderedRoot(t, "config.toml"), "config.toml"))
		if trustErr == nil {
			trustErr = toml.Unmarshal(data, &trust)
		}
	}
	var statuses []NativeStatus
	for _, spec := range m.Hooks {
		if spec.RawCommand == "" {
			continue
		}
		command := rawHookCommand(m, spec)
		prefix := strings.TrimSuffix(command, spec.RawCommand)
		wantEvent, wantMatcher := "", ""
		var wantHook claudeHookCmd
		for event, groups := range expected.Hooks {
			for _, group := range groups {
				for _, hook := range group.Hooks {
					if hook.Command == command {
						wantEvent, wantMatcher, wantHook = event, group.Matcher, hook
					}
				}
			}
		}
		if wantEvent == "" {
			return nil, fmt.Errorf("install: native %s was not rendered", spec.RawTag)
		}
		status := NativeStatus{Tag: spec.RawTag, State: "missing"}
		if t.Provider == agenthooks.ProviderCodex && (t.Scope == ScopeUser || t.Scope == ScopeProject) {
			status.Trust = "missing"
		}
		found := 0
		for event, groups := range actual.Hooks {
			for groupIndex, group := range groups {
				for handlerIndex, hook := range group.Hooks {
					if !strings.HasPrefix(hook.Command, prefix) {
						continue
					}
					found++
					status.State = "modified"
					if found == 1 && event == wantEvent && group.Matcher == wantMatcher && len(group.Hooks) == 1 && reflect.DeepEqual(hook, wantHook) {
						status.State = "ok"
					}
					if t.Provider == agenthooks.ProviderCodex && (t.Scope == ScopeUser || t.Scope == ScopeProject) {
						status.Trust = "missing"
						if trustErr != nil && !errors.Is(trustErr, os.ErrNotExist) {
							status.Trust = "unreadable"
						} else if entry, ok := trust.Hooks.State[codexTrustKey(m, t, event, groupIndex)]; ok && handlerIndex == 0 {
							status.Trust = "mismatch"
							if !hook.Async && entry.Hash == DefinitionHash(event, group.Matcher, hook.Command, hook.Timeout) {
								status.Trust = "match"
							}
						}
					}
				}
			}
		}
		if found > 1 {
			status.State = "modified"
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}
