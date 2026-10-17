package state

import (
	"context"
	"github.com/speakeasy-api/agenthooks/privatefs"
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
func TestM3A17WindowsPrivateRefreshDirectory(t *testing.T) {
	root := t.TempDir()
	if err := privatefs.PrepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := store.BeginRefresh(context.Background(), "CON")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err = privatefs.CheckPath(filepath.Join(root, component("CON"), "refresh.lock")); err != nil {
		t.Fatal(err)
	}
}
