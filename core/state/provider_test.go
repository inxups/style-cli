package state

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestM3A11ProviderOptionalStateSnapshotAndClear(t *testing.T) {
	root := t.TempDir()
	store, err := New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := store.Update("s", "main", EventUpdate{Model: "alias"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(session)
	if bytes.Contains(raw, []byte("provider")) {
		t.Fatal(string(raw))
	}
	session, _, err = store.Update("s", "main", EventUpdate{ModelProvider: "local.ID", ProviderResolved: true, ProviderSource: "codex-launch"})
	if err != nil || session.ModelProvider != "local.ID" {
		t.Fatal(session, err)
	}
	snapshot := filepath.Join(root, "snapshot")
	if _, err = CopySnapshot(context.Background(), store.root, snapshot, "s", "main"); err != nil {
		t.Fatal(err)
	}
	copied, _ := New(snapshot)
	got, _, err := copied.Read("s", "main")
	if err != nil || got.ModelProvider != "local.ID" || got.ProviderSource != "codex-launch" {
		t.Fatal(got, err)
	}
	cleared, _, err := store.Update("s", "main", EventUpdate{ProviderResolved: true, ProviderSource: "claude-provider-conflicting-selectors"})
	if err != nil || cleared.ModelProvider != "" || !cleared.ProviderResolved {
		t.Fatal(cleared, err)
	}
	before, err := os.ReadFile(filepath.Join(store.root, "s", "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Update("s", "main", EventUpdate{ProviderResolved: true, ModelProvider: "https://private.invalid", ProviderSource: "codex-launch"}); err == nil {
		t.Fatal("accepted endpoint")
	}
	after, _ := os.ReadFile(filepath.Join(store.root, "s", "session.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("invalid update changed state")
	}
	for _, s := range []Session{{ModelProvider: "local"}, {ProviderResolved: true, ModelProvider: "unknown", ProviderSource: "provided"}, {ProviderResolved: true, ModelProvider: "local", ProviderSource: "secret"}} {
		raw, _ := json.Marshal(s)
		if decodeState(raw, new(Session)) == nil {
			t.Fatal("accepted unsafe provider state")
		}
	}
}
