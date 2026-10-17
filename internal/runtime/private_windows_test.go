package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks/privatefs"
	"path/filepath"
	"testing"
)

func TestM3A17WindowsPrivateRefreshDirectory(t *testing.T) {
	root := t.TempDir()
	if err := privatefs.PrepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	store, err := state.New(root)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := store.BeginRefresh(context.Background(), "CON")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	component := fmt.Sprintf("!%x", sha256.Sum256([]byte("CON")))
	if err = privatefs.CheckPath(filepath.Join(root, component, "refresh.lock")); err != nil {
		t.Fatal(err)
	}
}
