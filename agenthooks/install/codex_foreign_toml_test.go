package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks"
)

func TestCodexNativeRegistrationInsideMarkersSurvivesLifecycle(t *testing.T) {
	requirePOSIXFixture(t, "POSIX plugin/project or author native fixture")
	for _, scope := range []Scope{ScopeUser, ScopePlugin} {
		t.Run(string(scope), func(t *testing.T) {
			root := t.TempDir()
			target := Target{Provider: agenthooks.ProviderCodex, Scope: scope, Dir: filepath.Join(root, "plugin"), ConfigDir: filepath.Join(root, "config")}
			if scope == ScopeUser {
				target.Dir = target.ConfigDir
			}
			manifest := testManifest()
			manifest.Owner = "style"
			manifest.Identity = Identity{Name: "style-runtime", Version: "0.1.0", PluginID: "style-runtime@style"}
			if err := Install(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(target.ConfigDir, "config.toml")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Codex's TOML editor inserts new tables before the trailing comment.
			foreign := []byte("[marketplaces.style]\nsource_type = \"local\"\nsource = \"/local/fixture\"\n\n[plugins.\"style-runtime@style\"]\nenabled = true\n\n")
			_, end := tomlMarkers("style")
			registered := bytes.Replace(before, []byte(end), append(append([]byte(nil), foreign...), []byte(end)...), 1)
			if err := os.WriteFile(path, registered, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := Install(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			unchanged, _ := os.ReadFile(path)
			if !bytes.Equal(unchanged, registered) {
				t.Fatal("idempotent install changed or removed native registration")
			}
			manifest.Command = []string{"/updated/style"}
			if err := Install(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			updated, _ := os.ReadFile(path)
			if !bytes.Contains(updated, foreign) {
				t.Fatal("hook update removed native registration")
			}
			planned, err := PlanUninstallFiles(manifest, target)
			if err != nil {
				t.Fatal(err)
			}
			if err := ApplyFiles(context.Background(), planned); err != nil {
				t.Fatal(err)
			}
			removed, _ := os.ReadFile(path)
			if !bytes.Contains(removed, foreign) || bytes.Contains(removed, []byte("trusted_hash")) {
				t.Fatalf("uninstall damaged foreign tables or retained trust: %s", removed)
			}
			var parsed map[string]any
			if err := toml.Unmarshal(removed, &parsed); err != nil {
				t.Fatalf("remaining foreign TOML is invalid: %v", err)
			}
		})
	}
}

func TestCodexForeignTOMLUsesParsedTablesAndRejectsBrokenRegions(t *testing.T) {
	begin, end := tomlMarkers("style")
	rendered := []byte(begin + "\n\n[hooks.state.\"owned\"]\ntrusted_hash=\"new\"\n\n" + end)
	foreign := "[plugins.\"quoted.name\"]\nnote='''\n[hooks.state.\"inside a string\"]\ntrusted_hash=\"keep this text\"\n'''\n\n[[marketplaces.local.entries]]\nname=\"retained\"\n\n"
	existing := []byte(begin + "\n\n[hooks.state.\"owned\"]\ntrusted_hash=\"old\"\n\n" + foreign + end)
	merged, err := mergeCodexManagedTOML(existing, rendered, "style")
	if err != nil || !bytes.Contains(merged, []byte(foreign)) || bytes.Contains(merged, []byte(`trusted_hash="old"`)) {
		t.Fatalf("merge = %s, %v", merged, err)
	}
	removed, changed, err := removeCodexManagedTOML(merged, "style")
	if err != nil || !changed || string(removed) != foreign {
		t.Fatalf("remove = %q, %t, %v", removed, changed, err)
	}
	for _, broken := range []string{
		begin + "\n[plugins.style]\nenabled=true\n",
		begin + "\n[plugins.style\nenabled=true\n" + end,
	} {
		if _, err := mergeCodexManagedTOML([]byte(broken), rendered, "style"); err == nil {
			t.Fatal("merge accepted a broken region")
		}
		if _, _, err := removeCodexManagedTOML([]byte(broken), "style"); err == nil {
			t.Fatal("uninstall accepted a broken region")
		}
	}
}
