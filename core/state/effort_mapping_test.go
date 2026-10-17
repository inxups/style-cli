package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestM3A13MappingStateSnapshotAndStrictProvenance(t *testing.T) {
	store, _ := New(t.TempDir())
	digest := "sha256:" + strings.Repeat("a", 64)
	approx := true
	session, _, err := store.Update("s", "main", EventUpdate{EffortMappingResolved: true, EffortMappingSHA256: digest, EffortMappingSource: "user", ThinkingEffort: "high", EffortRaw: "future", EffortApprox: &approx})
	if err != nil || session.EffortMappingSHA256 != digest {
		t.Fatal(session, err)
	}
	dest := filepath.Join(t.TempDir(), "snapshot")
	if _, err := CopySnapshot(context.Background(), store.root, dest, "s", "main"); err != nil {
		t.Fatal(err)
	}
	copied, _ := New(dest)
	read, _, err := copied.Read("s", "main")
	if err != nil || read.EffortMappingSource != "user" || read.EffortRaw != "future" {
		t.Fatal(read, err)
	}
	cleared, _, err := store.Update("s", "main", EventUpdate{EffortMappingResolved: true})
	if err != nil || cleared.ThinkingEffort != "" || cleared.EffortApprox != nil || cleared.EffortMappingSHA256 != "" {
		t.Fatal(cleared, err)
	}
	for _, bad := range []Session{{EffortMappingSource: "user"}, {EffortMappingSHA256: "private-canary", EffortMappingSource: "user"}, {EffortMappingSHA256: digest, EffortMappingSource: "private-canary"}} {
		data, _ := json.Marshal(bad)
		if decodeState(data, new(Session)) == nil {
			t.Fatal("invalid state provenance")
		}
	}
}
