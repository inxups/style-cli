package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks"
)

func TestProjectCodexTrustIsIsolatedBySource(t *testing.T) {
	user := t.TempDir()
	before := []byte("# original\nmodel = 'user'\n[projects.'/some/project']\ntrust_level = 'trusted'\n")
	if err := os.WriteFile(filepath.Join(user, "config.toml"), before, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Command: []string{"style", "--project-hook"}, Owner: "style", Hooks: []HookSpec{{Kind: agenthooks.KindToolPre, Blocking: true}, {NativeEvent: "PreToolUse", RawCommand: "printf native", RawTag: "native"}}}
	var targets []Target
	for i := 0; i < 2; i++ {
		dir := filepath.Join(t.TempDir(), ".codex")
		target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopeProject, Dir: dir, ConfigDir: user}
		targets = append(targets, target)
		files, err := PlanFiles(manifest, target)
		if err != nil {
			t.Fatal(err)
		}
		if err = ApplyFiles(context.Background(), files); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Join(dir, "config.toml")); !os.IsNotExist(err) {
			t.Fatal("trust written to project", err)
		}
		status, err := InspectNative(manifest, target)
		if err != nil || len(status) != 1 || status[0].Trust != "match" {
			t.Fatal(status, err)
		}
	}
	config := filepath.Join(user, "config.toml")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err = toml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "trusted_hash") != 4 {
		t.Fatal(string(data))
	}
	for _, target := range targets {
		files, err := PlanUninstallFiles(manifest, target)
		if err != nil {
			t.Fatal(err)
		}
		if err = ApplyFiles(context.Background(), files); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := os.ReadFile(config)
	if string(before) != string(after) {
		t.Fatalf("before=%q after=%q", before, after)
	}
	info, _ := os.Stat(config)
	if info.Mode().Perm() != 0o640 {
		t.Fatal(info.Mode())
	}
}
func TestProjectCodexTrustPreservesForeignTablesInsideRegion(t *testing.T) {
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopeProject, Dir: filepath.Join(t.TempDir(), ".codex"), ConfigDir: t.TempDir()}
	m := Manifest{Command: []string{"style"}, Owner: "style", Hooks: []HookSpec{{Kind: agenthooks.KindSessionStart, Blocking: true}}}
	files, err := PlanFiles(m, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyFiles(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target.ConfigDir, "config.toml")
	data, _ := os.ReadFile(path)
	_, end := tomlMarkers(codexTrustOwner(m, target))
	foreign := "[hooks.state.\"/foreign/hooks.json:session_start:0:0\"]\ntrusted_hash = 'foreign'\n"
	data = []byte(strings.Replace(string(data), end, foreign+end, 1))
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := PlanUninstallFiles(m, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyFiles(context.Background(), removed); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), foreign) {
		t.Fatal(string(after))
	}
}
func TestProjectDigestIgnoresForeignHandlersAndTracksOwnedChanges(t *testing.T) {
	for _, provider := range []agenthooks.Provider{agenthooks.ProviderCodex, agenthooks.ProviderClaudeCode} {
		t.Run(string(provider), func(t *testing.T) {
			target := Target{Provider: provider, Scope: ScopeProject, Dir: t.TempDir(), ConfigDir: t.TempDir()}
			m := Manifest{Command: []string{"style", "--project-hook"}, Owner: "style", Hooks: []HookSpec{{Kind: agenthooks.KindSessionStart, Blocking: true}}}
			files, err := PlanFiles(m, target)
			if err != nil {
				t.Fatal(err)
			}
			if err = ApplyFiles(context.Background(), files); err != nil {
				t.Fatal(err)
			}
			before, err := ProjectDigest(m, target, os.ReadFile)
			if err != nil {
				t.Fatal(err)
			}
			for path := range before {
				data, _ := os.ReadFile(path)
				var root map[string]any
				if err = json.Unmarshal(data, &root); err != nil {
					t.Fatal(err)
				}
				root["unrelated"] = true
				data, _ = json.Marshal(root)
				if err = os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			after, err := ProjectDigest(m, target, os.ReadFile)
			if err != nil {
				t.Fatal(err)
			}
			for path, digest := range before {
				if after[path] != digest {
					t.Fatal("foreign field changed owned digest")
				}
			}
		})
	}
}

func TestProjectDigestIncludesOwnedPartOfMixedGroups(t *testing.T) {
	for _, provider := range []agenthooks.Provider{agenthooks.ProviderCodex, agenthooks.ProviderClaudeCode} {
		t.Run(string(provider), func(t *testing.T) {
			target := Target{Provider: provider, Scope: ScopeProject, Dir: t.TempDir(), ConfigDir: t.TempDir()}
			m := Manifest{Command: []string{"style", "--project-hook"}, Owner: "style", Hooks: []HookSpec{{Kind: agenthooks.KindSessionStart, Blocking: true}}}
			files, err := PlanFiles(m, target)
			if err != nil {
				t.Fatal(err)
			}
			if err = ApplyFiles(context.Background(), files); err != nil {
				t.Fatal(err)
			}
			before, err := ProjectDigest(m, target, os.ReadFile)
			if err != nil {
				t.Fatal(err)
			}
			for path, digest := range before {
				data, _ := os.ReadFile(path)
				var root map[string]any
				if err = json.Unmarshal(data, &root); err != nil {
					t.Fatal(err)
				}
				hooks := root["hooks"].(map[string]any)
				for _, groups := range hooks {
					group := groups.([]any)[0].(map[string]any)
					group["hooks"] = append(group["hooks"].([]any), map[string]any{"type": "command", "command": "printf foreign"})
				}
				data, _ = json.Marshal(root)
				if err = os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				mixed, err := ProjectDigest(m, target, os.ReadFile)
				if err != nil || mixed[path] != digest {
					t.Fatal("foreign handler changed digest", mixed, err)
				}
				for _, groups := range hooks {
					group := groups.([]any)[0].(map[string]any)
					handler := group["hooks"].([]any)[0].(map[string]any)
					handler["timeout"] = 17
				}
				data, _ = json.Marshal(root)
				if err = os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				changed, err := ProjectDigest(m, target, os.ReadFile)
				if err != nil || changed[path] == digest {
					t.Fatal("owned handler change was ignored", changed, err)
				}
			}
		})
	}
}
