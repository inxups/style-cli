package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/agenthooks"
)

// PlannedFile is one exact filesystem mutation produced without writing.
// Path is the destination path, and Root is its owning install root. Before
// is nil when the file did not exist; a non-nil empty slice means it existed
// with zero bytes. After is nil when Remove is true.
type PlannedFile struct {
	Path   string
	Root   string
	Before []byte
	After  []byte
	Mode   fs.FileMode
	Remove bool
}

// PlanFiles returns the exact file changes Install would make without
// modifying the filesystem. Mergeable configuration files include the same
// owner-aware merge with existing user data that Install applies. Unchanged
// files are omitted.
func PlanFiles(m Manifest, t Target) ([]PlannedFile, error) {
	if t.Dir == "" {
		return nil, errors.New("install: Target.Dir is required")
	}
	if t.Provider == agenthooks.ProviderCodex && t.Scope == ScopePlugin && t.ConfigDir == "" {
		return nil, errors.New("install: Codex plugin scope requires Target.ConfigDir for trust state")
	}
	planned, err := plan(m, t)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedFile, 0, len(planned))
	for _, p := range planned {
		if p.state == StateUnchanged {
			continue
		}
		mode := p.mode
		if p.before != nil && mode&0o111 == 0 {
			// os.WriteFile preserves permissions on an existing non-executable
			// file, so expose that resulting mode rather than the renderer mode.
			if info, statErr := os.Stat(filepath.Join(p.root, filepath.FromSlash(p.path))); statErr == nil {
				mode = info.Mode().Perm()
			}
		}
		out = append(out, PlannedFile{
			Path:   filepath.Join(p.root, filepath.FromSlash(p.path)),
			Root:   p.root,
			Before: cloneBytes(p.before),
			After:  cloneBytes(p.final),
			Mode:   mode,
		})
	}
	return out, nil
}

// PlanUninstallFiles returns the exact owner-scoped changes Uninstall would
// make without modifying the filesystem. Files that would remain unchanged
// are omitted.
func PlanUninstallFiles(m Manifest, t Target) ([]PlannedFile, error) {
	if t.Dir == "" {
		return nil, errors.New("install: Target.Dir is required")
	}
	owner, err := ownerName(m)
	if err != nil {
		return nil, err
	}
	var rendered fs.FS
	if t.Provider == agenthooks.ProviderClaudeCode {
		rendered, err = renderClaudeForUninstall(m, t)
	} else {
		rendered, err = Render(m, t)
	}
	if err != nil {
		return nil, err
	}
	paths, contents, err := flatten(rendered)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedFile, 0, len(paths))
	for i, path := range paths {
		root := renderedRoot(t, path)
		dest := filepath.Join(root, filepath.FromSlash(path))
		existing, readErr := os.ReadFile(dest)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("install: read %s for uninstall: %w", path, readErr)
		}
		var updated []byte
		removeFile := false
		changed := false
		switch {
		case isMergeableJSON(path):
			updated, removeFile, changed, err = removeOwnedJSON(existing, owner)
		case isMergeableTOML(path):
			updated, changed = removeManagedTOML(existing, owner)
			removeFile = changed && len(bytes.TrimSpace(updated)) == 0
		default:
			if bytes.Equal(existing, contents[i]) {
				removeFile, changed = true, true
			}
		}
		if err != nil {
			return nil, fmt.Errorf("install: remove owner %q from %s: %w", owner, path, err)
		}
		if !changed {
			continue
		}
		out = append(out, PlannedFile{
			Path:   dest,
			Root:   root,
			Before: cloneBytes(existing),
			After:  cloneBytes(updated),
			Mode:   0o644,
			Remove: removeFile,
		})
	}
	return out, nil
}

// ApplyFiles applies a previously generated plan. It validates every planned
// file against its Before bytes before making any change, so stale plans do
// not overwrite intervening edits. A file already removed after planning is
// treated as complete for remove operations.
func ApplyFiles(ctx context.Context, changes []PlannedFile) error {
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.TrimSpace(change.Path) == "" {
			return errors.New("install: planned file path is required")
		}
		if err := validatePlannedPath(change); err != nil {
			return err
		}
		current, err := os.ReadFile(change.Path)
		exists := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("install: read %s before apply: %w", change.Path, err)
		}
		if change.Remove && !exists {
			continue
		}
		if exists != (change.Before != nil) || exists && !bytes.Equal(current, change.Before) {
			return fmt.Errorf("install: planned file changed before apply: %s", change.Path)
		}
		if !change.Remove && change.Mode == 0 {
			return fmt.Errorf("install: planned file mode is required: %s", change.Path)
		}
	}
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if change.Remove {
			if err := os.Remove(change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("install: remove %s: %w", change.Path, err)
			}
			if change.Root != "" {
				pruneEmptyDirs(change.Root, filepath.Dir(change.Path))
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(change.Path), 0o755); err != nil {
			return fmt.Errorf("install: %w", err)
		}
		if err := os.WriteFile(change.Path, change.After, change.Mode); err != nil {
			return fmt.Errorf("install: %w", err)
		}
		if change.Mode&0o111 != 0 {
			if err := os.Chmod(change.Path, change.Mode); err != nil {
				return fmt.Errorf("install: chmod %s: %w", change.Path, err)
			}
		}
	}
	return nil
}

// Uninstall removes one consumer's hook entries while preserving other owners
// and unrelated configuration. Generated standalone files are removed only
// when their bytes still match this manifest.
func Uninstall(ctx context.Context, m Manifest, t Target) error {
	planned, err := PlanUninstallFiles(m, t)
	if err != nil {
		return err
	}
	return ApplyFiles(ctx, planned)
}

func validatePlannedPath(change PlannedFile) error {
	if change.Root == "" {
		return nil
	}
	rel, err := filepath.Rel(change.Root, change.Path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("install: planned file %q escapes target root %q", change.Path, change.Root)
	}
	return nil
}

func cloneBytes(source []byte) []byte {
	if source == nil {
		return nil
	}
	return append([]byte{}, source...)
}
