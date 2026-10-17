package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/speakeasy-api/agenthooks"
)

func a8Target(t *testing.T) Target {
	t.Helper()
	root := t.TempDir()
	market := filepath.Join(root, "market ' space")
	return Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopePlugin, Dir: filepath.Join(market, "plugins/style-runtime"), ConfigDir: filepath.Join(root, "claude"), HarnessVersion: "2.1.301", LocalMarketplaceName: "style-" + strings.Repeat("a", 64), LocalMarketplaceDir: market}
}
func a8Manifest() Manifest {
	return Manifest{Command: []string{"/fixed ' path/style", "--style-home=/private ' home"}, Owner: "style", Identity: Identity{Name: "style-runtime", Version: "0.1.0", Description: "STYLE runtime hooks"}, Hooks: []HookSpec{{Kind: agenthooks.KindToolPre, Blocking: true}, {NativeEvent: "PreToolUse", RawCommand: "printf NATIVE", RawTag: "sample/native"}}}
}
func TestM3A8ClaudeLocalMarketplaceRenderingAndLifecycle(t *testing.T) {
	requirePOSIXFixture(t, "POSIX plugin/project or author native fixture")
	target := a8Target(t)
	m := a8Manifest()
	rendered, err := Render(m, target)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := fs.ReadFile(rendered, "scripts/style")
	if err != nil || !strings.Contains(string(launcher), `'\''`) {
		t.Fatal(string(launcher), err)
	}
	if _, err := fs.ReadFile(rendered, "bin/style"); !os.IsNotExist(err) {
		t.Fatal("top-level bin", err)
	}
	raw, err := fs.ReadFile(rendered, "hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("${CLAUDE_PLUGIN_ROOT}/scripts/style")) || !bytes.Contains(raw, []byte("printf NATIVE")) {
		t.Fatal(string(raw))
	}
	files, err := PlanFiles(m, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyFiles(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(target.Dir, "scripts/style"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatal(info, err)
	}
	marketplace := filepath.Join(target.LocalMarketplaceDir, ".claude-plugin/marketplace.json")
	original, err := os.ReadFile(marketplace)
	if err != nil || !bytes.Contains(original, []byte(`"source": "./plugins/style-runtime"`)) {
		t.Fatal(string(original), err)
	}
	if _, err = InspectRuntime(m, target, os.ReadFile); err != nil {
		t.Fatal(err)
	}
	// Unknown owner handlers survive a STYLE uninstall.
	hookPath := filepath.Join(target.Dir, "hooks/hooks.json")
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("hooks")
	}
	hooks := doc["hooks"].(map[string]any)
	hooks["PostToolUse"] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "other --owner=other"}}}}
	changed, _ := json.Marshal(doc)
	if err = os.WriteFile(hookPath, changed, 0o640); err != nil {
		t.Fatal(err)
	}
	files, err = PlanUninstallFiles(m, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyFiles(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	remaining, err := os.ReadFile(hookPath)
	if err != nil || !bytes.Contains(remaining, []byte("--owner=other")) || bytes.Contains(remaining, []byte("--owner=style")) {
		t.Fatal(string(remaining), err)
	}
	if _, err = os.Stat(marketplace); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestM3A8ClaudeRegistrationStates(t *testing.T) {
	for _, mode := range []string{"absent", "market-only", "enabled", "disabled", "wrong-source", "wrong-scope", "missing-enable", "partial-enable", "bad-json"} {
		t.Run(mode, func(t *testing.T) {
			target := a8Target(t)
			id := "style-runtime@" + target.LocalMarketplaceName
			files := map[string][]byte{}
			put := func(name string, value any) {
				b, _ := json.Marshal(value)
				files[filepath.Join(target.ConfigDir, name)] = b
			}
			known := map[string]any{target.LocalMarketplaceName: map[string]any{"source": map[string]string{"source": "directory", "path": target.LocalMarketplaceDir}, "installLocation": target.LocalMarketplaceDir}}
			if mode != "absent" {
				put("plugins/known_marketplaces.json", known)
			}
			if mode != "absent" && mode != "market-only" {
				put("plugins/installed_plugins.json", map[string]any{"version": 2, "plugins": map[string]any{id: []any{map[string]string{"scope": "user", "version": "0.1.0", "installPath": "/native/cache"}}}})
				put("settings.json", map[string]any{"enabledPlugins": map[string]bool{id: mode != "disabled"}})
			}
			switch mode {
			case "wrong-source":
				put("plugins/known_marketplaces.json", map[string]any{target.LocalMarketplaceName: map[string]any{"source": map[string]string{"source": "github", "path": "foreign"}, "installLocation": target.LocalMarketplaceDir}})
			case "wrong-scope":
				put("plugins/installed_plugins.json", map[string]any{"version": 2, "plugins": map[string]any{id: []any{map[string]string{"scope": "project", "version": "0.1.0"}}}})
			case "missing-enable":
				delete(files, filepath.Join(target.ConfigDir, "settings.json"))
			case "partial-enable":
				delete(files, filepath.Join(target.ConfigDir, "plugins/installed_plugins.json"))
			case "bad-json":
				files[filepath.Join(target.ConfigDir, "plugins/known_marketplaces.json")] = []byte("invalid")
			}
			before := map[string][]byte{}
			for name, b := range files {
				before[name] = append([]byte(nil), b...)
			}
			read := func(path string) ([]byte, error) {
				b, ok := files[path]
				if !ok {
					return nil, os.ErrNotExist
				}
				return b, nil
			}
			result, err := InspectClaudeRegistration(target, read)
			expected := "unknown"
			switch mode {
			case "absent", "market-only":
				expected = "registration-required"
			case "enabled":
				expected = "enabled"
			case "disabled":
				expected = "disabled"
			}
			if result.State != expected || (err != nil) != (expected == "unknown") || !reflect.DeepEqual(files, before) {
				t.Fatal(result, err)
			}
		})
	}
}
func TestM3A8ClaudePluginRuntimeRejectsChangedLauncher(t *testing.T) {
	requirePOSIXFixture(t, "POSIX plugin/project or author native fixture")
	target := a8Target(t)
	m := a8Manifest()
	files, err := PlanFiles(m, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyFiles(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target.Dir, "scripts/style")
	if err = os.WriteFile(path, []byte("foreign"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectRuntime(m, target, os.ReadFile); err == nil {
		t.Fatal("accepted foreign launcher")
	}
}
