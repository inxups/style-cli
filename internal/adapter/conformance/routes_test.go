package conformance_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

func TestRouteLifecycleRestoresForeignBytes(t *testing.T) {
	checkRouteLifecycle(t, false)
}

func TestNativeRouteLifecycleRestoresForeignBytes(t *testing.T) {
	checkRouteLifecycle(t, true)
}

func checkRouteLifecycle(t *testing.T, withNative bool) {
	t.Helper()
	for _, name := range []string{"codex-plugin", "codex-user", "claude-user"} {
		for _, ending := range []string{"", "\n", "\n\n"} {
			t.Run(name+"/ending="+string(rune('0'+len(ending))), func(t *testing.T) {
				if runtime.GOOS == "windows" && (withNative || name == "codex-plugin") {
					t.Skip("D11: POSIX plugin or author native fixture")
				}
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
				if withNative {
					var writer adapter.NativeWriter = codex.DefaultFacts()
					if name == "claude-user" {
						writer = claude.DefaultFacts()
					}
					for _, id := range []string{"first", "second"} {
						hook, err := writer.Native(spec.NativeMapping{Package: "fixture", HookID: id, Harness: string(provider), Event: "PreToolUse", Matcher: "Bash", Command: "printf native-" + id, Timeout: 17})
						if err != nil {
							t.Fatal(err)
						}
						hooks = append(hooks, hook)
					}
				}
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
				if withNative {
					inspector, ok := route.(adapter.NativeInspector)
					if !ok {
						t.Fatal("native route does not expose inspection")
					}
					statuses, err := inspector.InspectNative(context.Background(), hooks)
					if err != nil || len(statuses) != 2 {
						t.Fatalf("native installation=%+v err=%v", statuses, err)
					}
					for i, native := range statuses {
						if native.State != "ok" || native.Tag != "fixture/"+[]string{"first", "second"}[i] || name == "codex-user" && native.Trust != "match" {
							t.Fatalf("native status=%+v", native)
						}
					}
					path := filepath.Join(config, settingsName)
					if name == "codex-plugin" {
						path = filepath.Join(root, "plugin", "hooks", "hooks.json")
					}
					if count := bytes.Count(read(t, path), []byte("--native=fixture/")); count != 2 {
						t.Fatalf("native groups merged or duplicated: count=%d", count)
					}
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
				if withNative {
					statuses, err := route.(adapter.NativeInspector).InspectNative(context.Background(), hooks)
					if err != nil || len(statuses) != 2 {
						t.Fatalf("removed native status=%+v err=%v", statuses, err)
					}
					for _, native := range statuses {
						if native.State != "missing" {
							t.Fatalf("native removal residue: %+v", native)
						}
					}
				}
				for path, raw := range foreign {
					if got := read(t, path); !bytes.Equal(got, raw) {
						t.Errorf("foreign bytes changed in %s\ngot %q\nwant %q", filepath.Base(path), got, raw)
					}
					if info, err := os.Stat(path); err != nil || info.Mode().Perm() != fixtureForeignMode() {
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

func fixtureForeignMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return 0666
	}
	return 0640
}
