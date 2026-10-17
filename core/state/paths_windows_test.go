package state

import (
	"context"
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

func TestM3A17WindowsQuarantineHandleBehavior(t *testing.T) {
	store, _ := New(t.TempDir())
	guard, err := store.BeginRefresh(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	name := component("test")
	err = guard.root.Rename(name, "quarantined")
	t.Logf("directory rename with active child lock: %v", err)
	if err != nil {
		guard.file.Close()
		t.Logf("directory rename after child handle close: %v", guard.root.Rename(name, "quarantined"))
	}
}
