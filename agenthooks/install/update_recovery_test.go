package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type updateRecoveryContext struct {
	context.Context
	first   string
	mutate  func()
	reached bool
}

func (c *updateRecoveryContext) Err() error {
	data, err := os.ReadFile(c.first)
	if c.reached || err != nil || !bytes.Equal(data, []byte("after")) {
		return c.Context.Err()
	}
	c.reached = true
	if c.mutate != nil {
		c.mutate()
	}
	return context.Canceled
}

func TestM3A4FileRollbackPreservesModeAndMtime(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "remove"}[remove], func(t *testing.T) {
			root := t.TempDir()
			first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
			when := time.Date(2020, 1, 2, 3, 4, 5, 123456789, time.UTC)
			if err := os.WriteFile(first, []byte("before"), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(first, when, when); err != nil {
				t.Fatal(err)
			}
			original, _ := os.Stat(first)
			// Two validation + two staging checks precede applying either file.
			ctx := &cancelAfterChecks{Context: context.Background(), cancel: 6}
			files := []PlannedFile{{Root: root, Path: first, Before: []byte("before"), After: []byte("after"), Mode: 0o600, Remove: remove}, {Root: root, Path: second, After: []byte("never applied"), Mode: 0o600}}
			if err := ApplyFiles(ctx, files); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			info, err := os.Stat(first)
			data, readErr := os.ReadFile(first)
			if err != nil || readErr != nil || string(data) != "before" || info.Mode().Perm() != original.Mode().Perm() || !info.ModTime().Equal(original.ModTime()) {
				t.Fatalf("restoration=%v %v data=%q original=%v", info, err, data, original)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 || entries[0].Name() != "first" {
				t.Fatalf("debris=%v %v", entries, err)
			}
		})
	}
}

func TestM3A4FileRestoreFailureRetainsBackup(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	if err := os.WriteFile(first, []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	ctx := &updateRecoveryContext{Context: context.Background(), first: first, mutate: func() {
		if err := os.Remove(first); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(first, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(first, "obstruction"), []byte("intervening obstacle"), 0o600); err != nil {
			t.Fatal(err)
		}
	}}
	err := ApplyFiles(ctx, []PlannedFile{{Root: root, Path: first, Before: []byte("before"), After: []byte("after"), Mode: 0o600}, {Root: root, Path: second, After: []byte("not applied"), Mode: 0o600}})
	var recovery *RollbackError
	if err == nil || !ctx.reached || !errors.As(err, &recovery) || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "rollback failed:") || !strings.Contains(err.Error(), "backup retained at") {
		t.Fatalf("%v reached=%t", err, ctx.reached)
	}
	backups, err := filepath.Glob(filepath.Join(root, ".agenthooks-backup-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("retained backups=%v %v", backups, err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != "before" {
		t.Fatalf("backup=%q %v", data, err)
	}
	info, _ := os.Stat(backups[0])
	if info.Mode().Perm() != 0o640 {
		t.Fatal("backup lost original mode")
	}
	staged, _ := filepath.Glob(filepath.Join(root, ".agenthooks-stage-*"))
	if len(staged) != 0 {
		t.Fatal("uncommitted staging files were retained")
	}
}
