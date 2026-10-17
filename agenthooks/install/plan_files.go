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
	return planFilesWithReader(m, t, os.ReadFile)
}

func planFilesWithReader(m Manifest, t Target, readFile func(string) ([]byte, error)) ([]PlannedFile, error) {
	if t.Dir == "" {
		return nil, errors.New("install: Target.Dir is required")
	}
	if t.Provider == agenthooks.ProviderCodex && t.Scope == ScopePlugin && t.ConfigDir == "" {
		return nil, errors.New("install: Codex plugin scope requires Target.ConfigDir for trust state")
	}
	planned, err := planWithReader(m, t, readFile)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedFile, 0, len(planned))
	for _, p := range planned {
		if p.state == StateUnchanged {
			continue
		}
		mode := p.mode
		if p.before != nil && mode&0o111 == 0 && !strings.HasPrefix(p.path, ".agenthooks/") {
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
			if isMergeableJSON(path) {
				_, removal, err := removalJSONRestore(root, path, owner)
				if err != nil {
					return nil, err
				}
				if removal != nil {
					out = append(out, *removal)
				}
			}
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
			if err == nil {
				var record jsonRestore
				var removal *PlannedFile
				record, removal, err = removalJSONRestore(root, path, owner)
				if err == nil && removal != nil {
					out = append(out, *removal)
					if record.AppliedSHA256 == jsonDigest(existing) {
						updated = record.Original
						removeFile = updated == nil
						changed = removeFile || !bytes.Equal(existing, updated)
					}
				}
			}
		case isMergeableTOML(path):
			if t.Provider == agenthooks.ProviderCodex {
				updated, changed, err = removeCodexManagedTOML(existing, owner)
			} else {
				updated, changed = removeManagedTOML(existing, owner)
			}
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
		mode := fs.FileMode(0o644)
		if info, statErr := os.Stat(dest); statErr == nil {
			mode = info.Mode().Perm()
		}
		out = append(out, PlannedFile{
			Path:   dest,
			Root:   root,
			Before: cloneBytes(existing),
			After:  cloneBytes(updated),
			Mode:   mode,
			Remove: removeFile,
		})
	}
	return out, nil
}

// RollbackError reports an incomplete recovery while preserving the original
// failure for errors.Is/As. Backups named by Recovery remain available.
type RollbackError struct {
	Cause, Recovery error
}

func (e *RollbackError) Error() string   { return fmt.Sprintf("%v; %v", e.Cause, e.Recovery) }
func (e *RollbackError) Unwrap() []error { return []error{e.Cause, e.Recovery} }

// ApplyFiles applies a previously generated plan. It validates every planned
// file against its Before bytes before making any change, so stale plans do
// not overwrite intervening edits. A file already removed after planning is
// treated as complete for remove operations.
func ApplyFiles(ctx context.Context, changes []PlannedFile) error {
	if ctx == nil {
		return errors.New("install: context is required")
	}
	type stagedChange struct {
		change  PlannedFile
		temp    string
		backup  string
		applied bool
	}
	staged := make([]stagedChange, 0, len(changes))
	retained := make(map[string]bool)
	cleanup := func() {
		for _, item := range staged {
			if item.temp != "" {
				_ = os.Remove(item.temp)
			}
			if item.backup != "" && !retained[item.backup] {
				_ = os.Remove(item.backup)
			}
		}
	}
	rollback := func() error {
		var failures []string
		for index := len(staged) - 1; index >= 0; index-- {
			item := &staged[index]
			if !item.applied {
				continue
			}
			if item.change.Before == nil {
				if err := os.Remove(item.change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
					failures = append(failures, err.Error())
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(item.change.Path), 0o755); err != nil {
					retained[item.backup] = true
					failures = append(failures, fmt.Sprintf("backup retained at %s: %v", item.backup, err))
					continue
				}
				if err := os.Remove(item.change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
					retained[item.backup] = true
					failures = append(failures, fmt.Sprintf("backup retained at %s: %v", item.backup, err))
					continue
				}
				if err := os.Rename(item.backup, item.change.Path); err != nil {
					retained[item.backup] = true
					failures = append(failures, fmt.Sprintf("restore failed; backup retained at %s: %v", item.backup, err))
					continue
				}
				item.backup = ""
			}
		}
		cleanup()
		if len(failures) > 0 {
			return fmt.Errorf("rollback failed: %s", strings.Join(failures, "; "))
		}
		return nil
	}
	fail := func(cause error) error {
		if rollbackErr := rollback(); rollbackErr != nil {
			return &RollbackError{Cause: cause, Recovery: rollbackErr}
		}
		return cause
	}
	// Validate the full set before staging or mutating any destination.
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
	// Stage new content and rollback bytes beside each destination. The final
	// rename is atomic for each file; any later failure restores prior files.
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			cleanup()
			return err
		}
		if err := validatePlannedPath(change); err != nil {
			cleanup()
			return err
		}
		if change.Remove && change.Before == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(change.Path), 0o755); err != nil {
			cleanup()
			return fmt.Errorf("install: prepare %s: %w", change.Path, err)
		}
		item := stagedChange{change: change}
		if change.Before != nil {
			backup, err := os.CreateTemp(filepath.Dir(change.Path), ".agenthooks-backup-*")
			if err != nil {
				staged = append(staged, item)
				cleanup()
				return fmt.Errorf("install: stage rollback for %s: %w", change.Path, err)
			}
			item.backup = backup.Name()
			backupMode := change.Mode
			originalInfo, statErr := os.Stat(change.Path)
			if statErr != nil {
				backup.Close()
				staged = append(staged, item)
				cleanup()
				return statErr
			}
			backupMode = originalInfo.Mode().Perm()
			if err := backup.Chmod(backupMode); err != nil {
				backup.Close()
				staged = append(staged, item)
				cleanup()
				return err
			}
			if _, err := backup.Write(change.Before); err != nil {
				backup.Close()
				staged = append(staged, item)
				cleanup()
				return err
			}
			if err := backup.Sync(); err != nil {
				backup.Close()
				staged = append(staged, item)
				cleanup()
				return err
			}
			if err := backup.Close(); err != nil {
				staged = append(staged, item)
				cleanup()
				return err
			}
			if err := os.Chtimes(item.backup, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
				staged = append(staged, item)
				cleanup()
				return err
			}
		}
		if !change.Remove {
			temp, err := os.CreateTemp(filepath.Dir(change.Path), ".agenthooks-stage-*")
			if err != nil {
				staged = append(staged, item)
				cleanup()
				return fmt.Errorf("install: stage %s: %w", change.Path, err)
			}
			item.temp = temp.Name()
			if err := temp.Chmod(change.Mode); err != nil {
				temp.Close()
				staged = append(staged, item)
				cleanup()
				return err
			}
			if _, err := temp.Write(change.After); err != nil {
				temp.Close()
				staged = append(staged, item)
				cleanup()
				return err
			}
			if err := temp.Sync(); err != nil {
				temp.Close()
				staged = append(staged, item)
				cleanup()
				return err
			}
			if err := temp.Close(); err != nil {
				staged = append(staged, item)
				cleanup()
				return err
			}
		}
		staged = append(staged, item)
	}
	for index := range staged {
		item := &staged[index]
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		change := item.change
		if err := validatePlannedPath(change); err != nil {
			return fail(err)
		}
		current, err := os.ReadFile(change.Path)
		exists := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail(fmt.Errorf("install: recheck %s: %w", change.Path, err))
		}
		if change.Remove && !exists {
			continue
		}
		if exists != (change.Before != nil) || exists && !bytes.Equal(current, change.Before) {
			return fail(fmt.Errorf("install: planned file changed during apply: %s", change.Path))
		}
		if change.Remove {
			if err := os.Remove(change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fail(fmt.Errorf("install: remove %s: %w", change.Path, err))
			}
			if change.Root != "" {
				pruneEmptyDirs(change.Root, filepath.Dir(change.Path))
			}
			item.applied = true
			continue
		}
		if err := os.Rename(item.temp, change.Path); err != nil {
			return fail(fmt.Errorf("install: replace %s: %w", change.Path, err))
		}
		item.temp = ""
		item.applied = true
	}
	cleanup()
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
	return CheckPath(change.Root, change.Path)
}

// CheckPath checks containment and existing path components before a managed
// file is read or written. It rejects symlinks, including dangling links.
func CheckPath(root, path string) error {
	if root == "" {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("install: planned file %q escapes target root %q", path, root)
	}
	current := filepath.Clean(root)
	parts := strings.Split(rel, string(filepath.Separator))
	for i := -1; i < len(parts); i++ {
		if i >= 0 {
			current = filepath.Join(current, parts[i])
		}
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("install: inspect %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("install: symbolic link in managed path: %s", current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("install: managed parent is not a directory: %s", current)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("install: managed file is not regular: %s", current)
		}
	}
	return nil
}

func cloneBytes(source []byte) []byte {
	if source == nil {
		return nil
	}
	return append([]byte{}, source...)
}
