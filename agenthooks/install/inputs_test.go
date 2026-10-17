package install

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/speakeasy-api/agenthooks"
)

func TestM3A3ManagementPathsIncludesUnchangedFilesAndRestoreRecords(t *testing.T) {
	for _, scope := range []struct {
		provider agenthooks.Provider
		scope    Scope
	}{{agenthooks.ProviderCodex, ScopeUser}, {agenthooks.ProviderCodex, ScopePlugin}, {agenthooks.ProviderClaudeCode, ScopeUser}} {
		t.Run(string(scope.provider)+"/"+string(scope.scope), func(t *testing.T) {
			dir, config := t.TempDir(), t.TempDir()
			target := Target{Provider: scope.provider, Scope: scope.scope, Dir: dir, ConfigDir: config, HarnessVersion: "2.1.315"}
			m := testManifest()
			m.Owner = "style"
			m.Identity.PluginID = "myhooks@style"
			paths, err := ManagementPaths(m, target)
			if err != nil {
				t.Fatal(err)
			}
			planned, err := PlanFiles(m, target)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range planned {
				if !slices.Contains(paths, f.Path) {
					t.Fatalf("missing renderer input %s", f.Path)
				}
			}
			if err := ApplyFiles(context.Background(), planned); err != nil {
				t.Fatal(err)
			}
			unchanged, err := PlanFiles(m, target)
			if err != nil || len(unchanged) != 0 {
				t.Fatalf("%v %v", unchanged, err)
			}
			after, err := ManagementPaths(m, target)
			if err != nil || !slices.Equal(paths, after) {
				t.Fatalf("unchanged inputs lost: %v %v", after, err)
			}
			found := false
			for _, p := range paths {
				if filepath.Ext(p) == ".json" && filepath.Ext(p[:len(p)-5]) == ".restore" {
					found = true
				}
			}
			if !found {
				t.Fatal("owner restoration input omitted")
			}
		})
	}
}
