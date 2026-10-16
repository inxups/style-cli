package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Foreign matcher groups precede the newly rendered owner groups. Trust keys
// must address the merged layout, including during route transitions.
func rebaseCodexTrust(m Manifest, t Target, paths []string, contents [][]byte, readFile func(string) ([]byte, error)) error {
	hookPath := "hooks.json"
	if t.Scope == ScopePlugin {
		hookPath = "hooks/hooks.json"
	}
	configIndex, hooksIndex := -1, -1
	for i, path := range paths {
		switch filepath.ToSlash(path) {
		case "config.toml":
			configIndex = i
		case hookPath:
			hooksIndex = i
		}
	}
	if configIndex < 0 || hooksIndex < 0 {
		return nil
	}
	existing, err := readFile(filepath.Join(t.Dir, filepath.FromSlash(hookPath)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var before, rendered struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(existing, &before); err != nil {
		return fmt.Errorf("install: read Codex hook groups: %w", err)
	}
	if err := json.Unmarshal(contents[hooksIndex], &rendered); err != nil {
		return err
	}
	var replacements []string
	for event, groups := range rendered.Hooks {
		foreign, err := removeOwnedHookGroups(before.Hooks[event], manifestOwner(m))
		if err != nil {
			return err
		}
		if len(foreign) == 0 {
			continue
		}
		for i := range groups {
			replacements = append(replacements, tomlString(codexTrustKey(m, t, event, i)), tomlString(codexTrustKey(m, t, event, len(foreign)+i)))
		}
	}
	if len(replacements) > 0 {
		contents[configIndex] = []byte(strings.NewReplacer(replacements...).Replace(string(contents[configIndex])))
	}
	return nil
}
