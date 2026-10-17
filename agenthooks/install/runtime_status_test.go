package install

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

func a2RuntimeConfig(t *testing.T, provider agenthooks.Provider, plugin bool) (Manifest, Target, map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	config := dir
	scope := ScopeUser
	if plugin {
		scope = ScopePlugin
		config = t.TempDir()
	}
	manifest := Manifest{Command: []string{"/fixture/style", "--style-home=/fixture/home", "--harness-home=" + config}, Owner: "style", Identity: Identity{Name: "style-runtime", Version: "0.1.0", PluginID: "style-runtime@style"}, Hooks: []HookSpec{{Kind: agenthooks.KindToolPre, NativeMatcher: "Bash", Blocking: true, Timeout: 10 * time.Second}, {NativeEvent: "SessionEnd", RawCommand: "echo native-canary", RawTag: "author"}}}
	target := Target{Provider: provider, Scope: scope, Dir: dir, ConfigDir: config, HarnessVersion: "2.1.301"}
	rendered, err := Render(manifest, target)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	err = fs.WalkDir(rendered, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := fs.ReadFile(rendered, path)
			if err != nil {
				return err
			}
			root := dir
			if plugin && path == "config.toml" {
				root = config
			}
			files[filepath.Join(root, filepath.FromSlash(path))] = data
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if plugin {
		path := filepath.Join(config, "config.toml")
		files[path] = append(files[path], []byte("\n\n[plugins.\"style-runtime@style\"]\nenabled = true\n")...)
	}
	return manifest, target, files
}
func a2ConfigReader(files map[string][]byte) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		data, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return data, nil
	}
}
func TestM3A2RuntimeOwnedSubscriptionInspection(t *testing.T) {
	for _, name := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(name, func(t *testing.T) {
			provider := agenthooks.ProviderCodex
			if name == "claude-user" {
				provider = agenthooks.ProviderClaudeCode
			}
			manifest, target, files := a2RuntimeConfig(t, provider, name == "codex-plugin")
			status, err := InspectRuntime(manifest, target, a2ConfigReader(files))
			if err != nil || len(status.Events) != 1 || status.Events[0] != "PreToolUse" {
				t.Fatalf("runtime/native separation: %+v %v", status, err)
			}
		})
	}
}
func TestM3A2RuntimeConfigurationDriftAndNativeOnly(t *testing.T) {
	for _, mode := range []string{"command", "args", "type", "extra", "mixed-group", "trust", "plugin-disabled", "launcher", "descriptor", "bad-json", "foreign", "native-only"} {
		t.Run(mode, func(t *testing.T) {
			plugin := mode == "plugin-disabled" || mode == "launcher" || mode == "descriptor"
			provider := agenthooks.ProviderCodex
			if mode == "args" || mode == "native-only" {
				provider = agenthooks.ProviderClaudeCode
			}
			manifest, target, files := a2RuntimeConfig(t, provider, plugin)
			path := filepath.Join(target.Dir, "hooks.json")
			if plugin {
				path = filepath.Join(target.Dir, "hooks", "hooks.json")
			}
			if provider == agenthooks.ProviderClaudeCode {
				path = filepath.Join(target.Dir, "settings.json")
			}
			var document map[string]any
			if err := json.Unmarshal(files[path], &document); err != nil {
				t.Fatal(err)
			}
			hooks := document["hooks"].(map[string]any)
			groups := hooks["PreToolUse"].([]any)
			group := groups[0].(map[string]any)
			handlers := group["hooks"].([]any)
			handler := handlers[0].(map[string]any)
			switch mode {
			case "command":
				handler["command"] = strings.Replace(handler["command"].(string), "/fixture/style", "/other/style", 1)
			case "args":
				handler["args"] = []string{"agenthooks", "run", "--owner=style", "--provider=claude-code", "--style-home=/other"}
			case "type":
				handler["type"] = "prompt"
			case "extra":
				handler["prompt"] = "canary"
			case "mixed-group":
				group["hooks"] = append(handlers, map[string]any{"type": "command", "command": "foreign"})
			case "trust":
				key := filepath.Join(target.Dir, "config.toml")
				files[key] = []byte("[hooks.state]\n")
			case "plugin-disabled":
				key := filepath.Join(target.ConfigDir, "config.toml")
				files[key] = []byte(strings.Replace(string(files[key]), "enabled = true", "enabled = false", 1))
			case "launcher":
				files[filepath.Join(target.Dir, "bin", "style")] = []byte("#!/bin/sh\necho unrelated\n")
			case "descriptor":
				files[filepath.Join(target.Dir, ".codex-plugin", "plugin.json")] = []byte(`{"name":"other"}`)
			case "bad-json":
				files[path] = []byte("{")
			case "foreign":
				hooks["SessionStart"] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo foreign --owner=other"}}}}
			case "native-only":
				delete(hooks, "PreToolUse")
			}
			if mode != "bad-json" {
				data, _ := json.Marshal(document)
				files[path] = data
			}
			status, err := InspectRuntime(manifest, target, a2ConfigReader(files))
			if mode == "foreign" {
				if err != nil || len(status.Events) != 1 {
					t.Fatal(status, err)
				}
			} else if mode == "native-only" {
				if err != nil || len(status.Events) != 0 {
					t.Fatal(status, err)
				}
			} else if err == nil {
				t.Fatalf("configuration drift accepted: %s", mode)
			}
		})
	}
}

func TestM3A2ClaudeActualSubscriptionTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, 250 * time.Millisecond, 1500 * time.Millisecond, 10 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			manifest, target, _ := a2RuntimeConfig(t, agenthooks.ProviderClaudeCode, false)
			manifest.Hooks = manifest.Hooks[:1]
			manifest.Hooks[0].Timeout = timeout
			manifest.Hooks = append(manifest.Hooks, HookSpec{Kind: agenthooks.KindSessionStart, Blocking: true})
			rendered, err := Render(manifest, target)
			if err != nil {
				t.Fatal(err)
			}
			data, err := fs.ReadFile(rendered, "settings.json")
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{filepath.Join(target.Dir, "settings.json"): data}
			// The consumer knows its command, while subscriptions are obtained
			// exclusively from its existing owned configuration.
			manifest.Hooks = nil
			status, err := InspectRuntime(manifest, target, a2ConfigReader(files))
			if err != nil || len(status.Events) != 2 || status.Events[0] != "PreToolUse" || status.Events[1] != "SessionStart" {
				t.Fatalf("actual subscription: %+v %v", status, err)
			}
		})
	}
}
