package install

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/agenthooks"
)

// Each project owns its trust region independently of the shared runner owner.
// Zero-value/user/plugin targets retain their original markers and bytes.
func codexTrustOwner(m Manifest, t Target) string {
	if t.Scope != ScopeProject {
		return manifestOwner(m)
	}
	digest := sha256.Sum256([]byte(filepath.Join(canonicalCodexDir(t.Dir), "hooks.json")))
	return fmt.Sprintf("%s-project-%x", manifestOwner(m), digest)
}

// ProjectDigest hashes this owner's normalized hook groups, excluding foreign
// handlers and the user trust file. This is portable clone evidence, not trust.
func ProjectDigest(m Manifest, t Target, read func(string) ([]byte, error)) (map[string]string, error) {
	rendered, err := Render(m, t)
	if err != nil {
		return nil, err
	}
	paths, _, err := flatten(rendered)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, name := range paths {
		if !isMergeableJSON(name) {
			continue
		}
		data, err := read(filepath.Join(t.Dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		var root struct {
			Hooks map[string][]json.RawMessage `json:"hooks"`
		}
		if err = json.Unmarshal(data, &root); err != nil {
			return nil, err
		}
		owned := map[string][]json.RawMessage{}
		for event, groups := range root.Hooks {
			for _, group := range groups {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(group, &fields); err != nil {
					return nil, err
				}
				var handlers []json.RawMessage
				if err := json.Unmarshal(fields["hooks"], &handlers); err != nil {
					return nil, err
				}
				selected := []json.RawMessage{}
				for _, raw := range handlers {
					var h struct {
						Command string   `json:"command"`
						Args    []string `json:"args"`
					}
					if err := json.Unmarshal(raw, &h); err != nil {
						return nil, err
					}
					if handlerHasOwner(h.Command, h.Args, manifestOwner(m)) {
						selected = append(selected, raw)
					}
				}
				if len(selected) == 0 {
					continue
				}
				fields["hooks"], err = json.Marshal(selected)
				if err != nil {
					return nil, err
				}
				raw, err := json.Marshal(fields)
				if err != nil {
					return nil, err
				}
				owned[event] = append(owned[event], raw)
			}
		}
		raw, err := json.Marshal(owned)
		if err != nil {
			return nil, err
		}
		var normalized any
		if err = json.Unmarshal(raw, &normalized); err != nil {
			return nil, err
		}
		raw, err = json.Marshal(normalized)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(raw)
		result[filepath.Join(t.Dir, filepath.FromSlash(name))] = fmt.Sprintf("sha256:%x", digest)
	}
	return result, nil
}

func codexManagedSources(t Target) []string {
	if t.Scope != ScopeProject {
		return nil
	}
	return []string{filepath.Join(canonicalCodexDir(t.Dir), "hooks.json")}
}

// UserOwner detects a pre-existing owner even when a consumer's selected
// storage root has no user lock. The consumer captures every read for drift.
func UserOwner(provider agenthooks.Provider, root, owner string, read func(string) ([]byte, error)) (bool, error) {
	name := "hooks.json"
	if provider == agenthooks.ProviderClaudeCode {
		name = "settings.json"
	}
	data, err := read(filepath.Join(root, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil {
		_, _, changed, err := removeOwnedJSON(data, owner)
		if err != nil {
			return false, err
		}
		if changed {
			return true, nil
		}
	}
	if provider == agenthooks.ProviderCodex {
		data, err = read(filepath.Join(root, "config.toml"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		begin, end := tomlMarkers(owner)
		return strings.Contains(string(data), begin) || strings.Contains(string(data), end), nil
	}
	return false, nil
}
