package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPathRejectsSymlinksAndNonregularComponents(t *testing.T) {
	for _, kind := range []string{"root-link", "parent-link", "file-link", "dangling-link", "parent-file", "directory", "escape"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "root")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "rules", "style-first.md")
			outside := filepath.Join(base, "outside")
			if err := os.WriteFile(outside, []byte("external"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "root-link":
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(base, root); err != nil {
					t.Fatal(err)
				}
			case "parent-link":
				if err := os.Symlink(base, filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			case "file-link", "dangling-link":
				if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				target := outside
				if kind == "dangling-link" {
					target += ".missing"
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "parent-file":
				if err := os.WriteFile(filepath.Dir(path), []byte("file"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "escape":
				path = outside
			}
			if err := CheckPath(root, path); err == nil {
				t.Fatal("unsafe managed path accepted")
			}
			if err := ApplyFiles(context.Background(), []PlannedFile{{Root: root, Path: path, After: []byte("new"), Mode: 0o644}}); err == nil {
				t.Fatal("unsafe file applied")
			}
			if data, err := os.ReadFile(outside); err != nil || string(data) != "external" {
				t.Fatal("external target overwritten")
			}
		})
	}
}

type pathMutationContext struct {
	context.Context
	checks int
	mutate func()
}

func (c *pathMutationContext) Err() error {
	c.checks++
	if c.checks == 6 {
		c.mutate()
	}
	return nil
}

func TestApplyFilesRechecksPathAndRollsBackPriorMutation(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	outside := filepath.Join(t.TempDir(), "outside")
	for _, path := range []string{first, outside} {
		if err := os.WriteFile(path, []byte("original"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &pathMutationContext{Context: context.Background(), mutate: func() {
		if err := os.Symlink(outside, second); err != nil {
			t.Fatal(err)
		}
	}}
	err := ApplyFiles(ctx, []PlannedFile{{Root: dir, Path: first, Before: []byte("original"), After: []byte("updated"), Mode: 0o600}, {Root: dir, Path: second, After: []byte("created"), Mode: 0o644}})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("path changed after staging: %v", err)
	}
	for _, path := range []string{first, outside} {
		if data, err := os.ReadFile(path); err != nil || string(data) != "original" {
			t.Fatal("prior bytes not restored")
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != fixtureMode(0o640) {
			t.Fatal("prior permissions not restored")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".agenthooks-") {
			t.Fatal("staging debris remains")
		}
	}
}
