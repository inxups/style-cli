package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRefreshCleanupPreservesActiveLock(t *testing.T) {
	s := refreshStore(t)
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", nil)
	g, err := s.BeginRefresh(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	old := time.Now().Add(-2 * DefaultExpiry)
	err = filepath.WalkDir(filepath.Join(s.root, component("s")), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, old, old)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.Cleanup(DefaultExpiry); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if err := g.Check(); err != nil {
		t.Fatal("cleanup disturbed an active guard", err)
	}
	g.Close()
	if n, err := s.Cleanup(DefaultExpiry); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(filepath.Join(s.root, component("s"))); !os.IsNotExist(err) {
		t.Fatal("expired session survived", err)
	}
}

func TestRefreshStrictRequiredStateFields(t *testing.T) {
	for _, data := range []string{
		`{"format":1,"gen":0,"turns":{},"cursors":{}}`,
		`{"format":1,"gen":null,"q":0,"turns":{},"cursors":{}}`,
		`{"format":1,"gen":0,"q":0,"turns":{},"cursors":{"0000000000000000000000000000000000000000000000000000000000000000":{"reservation":"00000000000000000000000000000000"}}}`,
	} {
		if _, err := decodeRefreshState([]byte(data)); err == nil {
			t.Fatalf("accepted incomplete state: %s", data)
		}
	}
}

func TestRefreshCleanupFirstEnableAndLegacyCandidates(t *testing.T) {
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	s := refreshStore(t)
	if _, _, err := s.Update("recent", "main", EventUpdate{}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Cleanup(DefaultExpiry); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(filepath.Join(s.root, component("recent"), "refresh.lock")); !os.IsNotExist(err) {
		t.Fatal("cleanup touched a recent legacy session", err)
	}
	if _, _, err := s.Update("old", "main", EventUpdate{}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * DefaultExpiry)
	if err := filepath.WalkDir(filepath.Join(s.root, component("old")), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		t.Fatal(err)
	}
	// First enable owns the guard before it has created any refresh clock.
	g, err := s.BeginRefresh(context.Background(), "old")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	os.Chtimes(filepath.Join(s.root, component("old"), "refresh.lock"), old, old)
	os.Chtimes(filepath.Join(s.root, component("old")), old, old)
	if n, err := s.Cleanup(DefaultExpiry); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if err := g.Check(); err != nil {
		t.Fatal("first enable lost its inode", err)
	}
	g.Close()
	if n, err := s.Cleanup(DefaultExpiry); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}

func TestRefreshWriteAndRollbackFailuresPreservePriorClaims(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory chmod fault; Windows DACL faults have separate coverage")
	}
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	s := refreshStore(t)
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) { refreshClaim(t, f, 1, "old", true) })
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, "one", func(f *RefreshFrame) {
		dir := filepath.Dir(filepath.Join(s.root, f.path))
		before, err := os.ReadFile(filepath.Join(s.root, f.path))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, 0700)
		for _, body := range []string{"new", "old"} {
			_, allowed, err := s.Claim(ClaimInput{Scope: f.scope, Repeat: "session", Action: "inject", Event: "tool.pre", Rule: "pkg/r", Body: body, ReinjectOnCompact: true, Refresh: 1, RefreshFrame: f})
			if err == nil || allowed {
				t.Fatal("write failure accepted", allowed, err)
			}
			after, _ := os.ReadFile(filepath.Join(s.root, f.path))
			if string(after) != string(before) {
				t.Fatal("failed save changed cursor")
			}
		}
		os.Chmod(dir, 0700)
		r, _ := refreshClaim(t, f, 1, "old", true)
		os.Chmod(dir, 0500)
		if err := r.Release(); err == nil {
			t.Fatal("rollback failure ignored")
		}
		os.Chmod(dir, 0700)
		if err := r.Release(); err != nil {
			t.Fatal("rollback retry failed", err)
		}
		_, trace := refreshClaim(t, f, 1, "old", true)
		if trace.Last == nil || *trace.Last != 0 {
			t.Fatal(trace)
		}
		refreshClaim(t, f, 1, "new", true) // New ordinary claim from the failed attempt was removed.
	})
}

func TestRefreshStaleReleaseDoesNotUndoOtherInvocation(t *testing.T) {
	s := refreshStore(t)
	var first *Reservation
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", func(f *RefreshFrame) { first, _ = refreshClaim(t, f, 1, "A", true) })
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{PromptSubmitted: true}, "1", func(f *RefreshFrame) { refreshClaim(t, f, 1, "A", true) })
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "1", func(f *RefreshFrame) {
		_, tr := refreshClaim(t, f, 1, "A", false)
		if tr.Last == nil || *tr.Last != 1 {
			t.Fatal(tr)
		}
	})
}

func TestRefreshSnapshotCombinedFileAndByteLimits(t *testing.T) {
	if !RefreshSupported() {
		t.Skip("refresh platform")
	}
	t.Run("files", func(t *testing.T) {
		s := refreshStore(t)
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", nil)
		dir := filepath.Join(s.root, component("s"), component("main"), "seen")
		os.Mkdir(dir, 0700)
		for i := 0; i < 9998; i++ {
			name := fmt.Sprintf("s.0.0.%016x.%016x", i, i)
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := CopySnapshot(context.Background(), s.root, t.TempDir(), "s", "main"); err == nil {
			t.Fatal("combined 10001 files accepted")
		}
	})
	t.Run("bytes", func(t *testing.T) {
		s := refreshStore(t)
		refreshEvent(t, s, "s", "main", "codex", EventUpdate{}, "", nil)
		v := refreshState{Format: 1, Turns: map[string]uint64{}, Cursors: map[string]refreshCursor{}}
		for i := 0; i < 25000; i++ {
			v.Cursors[fmt.Sprintf("%064x", i)] = refreshCursor{Reservation: "00000000000000000000000000000000"}
		}
		data, err := json.Marshal(v)
		if err != nil || len(data) > maxRefreshBytes {
			t.Fatal(len(data), err)
		}
		for i := 0; i < 6; i++ {
			if err := os.WriteFile(filepath.Join(s.root, component("s"), component("main"), "refresh", fmt.Sprintf("h%d.json", i)), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := CopySnapshot(context.Background(), s.root, t.TempDir(), "s", "main"); err == nil {
			t.Fatal("combined byte limit bypassed")
		}
	})
}
