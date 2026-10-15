package conformance_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

func TestRouteLifecycleRestoresForeignBytes(t *testing.T) {
	for _, name := range []string{"codex-plugin", "codex-user", "claude-user"} {
		for _, ending := range []string{"", "\n", "\n\n"} {
			t.Run(name+"/ending="+string(rune('0'+len(ending))), func(t *testing.T) {
				root := t.TempDir()
				config := filepath.Join(root, "config")
				if err := os.Mkdir(config, 0o700); err != nil {
					t.Fatal(err)
				}
				provider := agenthooks.ProviderCodex
				settingsName := "hooks.json"
				if name == "claude-user" {
					provider, settingsName = agenthooks.ProviderClaudeCode, "settings.json"
				}
				foreign := map[string][]byte{
					filepath.Join(config, settingsName): []byte("{\n \"foreign\" : \"\\u4fdd\\u7559\", \"hooks\" : {\"PreToolUse\" : [ {\"matcher\":\"Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"foreign --owner=other\"}]} ]}\n}" + ending),
				}
				var route adapter.Route
				if name == "claude-user" {
					foreign[filepath.Join(config, "CLAUDE.md")] = []byte("User instructions." + ending)
					route = claude.NewUserRouteForInstallation(config, "2.1.315", "/tmp/style", nil)
				} else {
					foreign[filepath.Join(config, "config.toml")] = []byte("# User config\nmodel = 'keep'" + ending)
					route = codex.NewUserRouteForCommand(config, "/tmp/style", nil)
					if name == "codex-plugin" {
						route = codex.NewPluginRouteForCommand(filepath.Join(root, "plugin"), config, "/tmp/style", nil)
					}
				}
				for path, raw := range foreign {
					if err := os.WriteFile(path, raw, 0o640); err != nil {
						t.Fatal(err)
					}
				}
				installation := adapter.Installation{Provider: provider, ConfigDir: config, Version: "2.1.315"}
				hooks := []adapter.HookSpec{{NativeEvent: "PreToolUse", Matcher: "Bash", Points: []string{"tool.pre"}}}
				blocks := []adapter.StaticBlock{{Owner: "fixture", Body: "Managed context."}}
				changes, err := route.Plan(context.Background(), hooks, blocks)
				if err != nil {
					t.Fatal(err)
				}
				for path, raw := range foreign {
					if !bytes.Equal(read(t, path), raw) {
						t.Fatal("Plan wrote configuration")
					}
				}
				if err := route.Apply(context.Background(), changes); err != nil {
					t.Fatal(err)
				}
				status, err := route.Status(context.Background(), installation)
				if err != nil || !status.Available || !status.Installed {
					t.Fatalf("installed status=%#v error=%v", status, err)
				}
				second, err := route.Plan(context.Background(), hooks, blocks)
				if err != nil || len(second.Files) != 0 {
					t.Fatalf("non-idempotent install: %#v %v", second, err)
				}
				removed, err := route.Remove(context.Background(), "style")
				if err != nil {
					t.Fatal(err)
				}
				if err := route.Apply(context.Background(), removed); err != nil {
					t.Fatal(err)
				}
				status, err = route.Status(context.Background(), installation)
				if err != nil || status.Installed {
					t.Fatalf("removed status=%#v error=%v", status, err)
				}
				for path, raw := range foreign {
					if got := read(t, path); !bytes.Equal(got, raw) {
						t.Errorf("foreign bytes changed in %s\ngot %q\nwant %q", filepath.Base(path), got, raw)
					}
					if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
						t.Errorf("foreign mode changed: %v %v", info, err)
					}
				}
				if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
					if err == nil && !entry.IsDir() {
						if _, ok := foreign[path]; !ok {
							t.Errorf("unexpected file after Remove: %s", path)
						}
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				again, err := route.Remove(context.Background(), "style")
				if err != nil || len(again.Files) != 0 {
					t.Fatalf("non-idempotent removal: %#v %v", again, err)
				}
			})
		}
	}
}
