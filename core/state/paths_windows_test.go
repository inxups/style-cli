package state

import (
	"context"
	"github.com/inxups/style-cli/internal/saferead"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM3A17WindowsSessionComponentsAvoidAliases(t *testing.T) {
	seen := map[string]bool{}
	for _, session := range []string{"CON", "con", "NUL", "AUX", "COM1", "LPT1", "session", "SESSION", `C:relative`, `a:b`, `a.`, "中文 空格"} {
		component := component(session)
		if !filepath.IsLocal(component) || seen[strings.ToLower(component)] {
			t.Fatal("device name or case collision", session, component)
		}
		seen[strings.ToLower(component)] = true
	}
}

// The gate must cover the interval after cleanup closes the session child
// handle. Otherwise a new process could write into the quarantined session.
func TestM3A17WindowsCleanupGateExcludesRefresh(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.BeginRefresh(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.BeginRefresh(context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	if gate, err := platformCleanupGate(store.root); err == nil {
		gate.Close()
		t.Fatal("cleanup acquired gate held by writers")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if gate, err := platformCleanupGate(store.root); err == nil {
		gate.Close()
		t.Fatal("cleanup ignored second writer")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	gate, err := platformCleanupGate(store.root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unlockRefresh(gate); gate.Close() }()
	if guard, err := store.BeginRefresh(context.Background(), "first"); err == nil {
		guard.Close()
		t.Fatal("writer acquired exclusive cleanup gate")
	}
	root, err := saferead.AnchorMutable(store.root)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := openRefreshGuardGated(context.Background(), store, root, component("first"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err := quarantineRefreshSession(guard, "quarantined"); err != nil {
		t.Fatal(err)
	}
	if guard.file != nil {
		t.Fatal("quarantine retained child lock handle")
	}
	if _, err := os.Stat(filepath.Join(store.root, "quarantined", "refresh.lock")); err != nil {
		t.Fatal(err)
	}
}
func TestM3A17WindowsCleanupGateRejectsCorruptionAndCancellation(t *testing.T) {
	for _, scenario := range []string{"nonempty", "directory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			root, err := saferead.AnchorMutable(path)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			ctx := context.Background()
			switch scenario {
			case "nonempty":
				if err := os.WriteFile(filepath.Join(path, refreshGateName), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(filepath.Join(path, refreshGateName), 0700); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			gate, _, err := platformRefreshGate(ctx, root, true, true)
			if gate != nil {
				unlockRefresh(gate)
				gate.Close()
			}
			if err == nil {
				t.Fatal("invalid gate accepted")
			}
		})
	}
}
