package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// PlanFilesAfter plans an installation against earlier changes in memory and
// returns one combined changeset, retaining the original Before snapshots.
// This lets routes sharing a configuration file switch in one transaction.
// Neither step is written until ApplyFiles validates and applies the result.
func PlanFilesAfter(m Manifest, t Target, previous []PlannedFile) ([]PlannedFile, error) {
	byPath := make(map[string]PlannedFile, len(previous))
	for _, change := range previous {
		path := filepath.Clean(change.Path)
		if _, exists := byPath[path]; exists {
			return nil, fmt.Errorf("install: duplicate earlier change %s", path)
		}
		change.Path = path
		byPath[path] = change
	}
	read := func(path string) ([]byte, error) {
		if change, ok := byPath[filepath.Clean(path)]; ok {
			if change.Remove {
				return nil, os.ErrNotExist
			}
			return cloneBytes(change.After), nil
		}
		return os.ReadFile(path)
	}
	next, err := planFilesWithReader(m, t, read)
	if err != nil {
		return nil, err
	}
	for _, change := range next {
		path := filepath.Clean(change.Path)
		if earlier, exists := byPath[path]; exists {
			change.Before = earlier.Before
		}
		byPath[path] = change
	}
	result := make([]PlannedFile, 0, len(byPath))
	for _, change := range byPath {
		if !change.Remove && bytes.Equal(change.Before, change.After) {
			info, err := os.Stat(change.Path)
			if err == nil && info.Mode().Perm() == change.Mode {
				continue
			}
		}
		result = append(result, change)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
