package install

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestM3A2RegisteredCodexCache(t *testing.T) {
	for _, mode := range []string{"valid", "missing-hooks", "missing-launcher", "missing-descriptor", "stale-hooks", "stale-launcher", "stale-descriptor", "remote", "relative-root", "wrong-source", "remote-plugin", "wrong-marketplace", "duplicate", "missing-plugin", "missing-catalog", "bad-catalog", "unsafe-version"} {
		t.Run(mode, func(t *testing.T) {
			m, target, files := a2RuntimeConfig(t, "codex", true)
			marketplace := t.TempDir()
			kind, root := "local", marketplace
			if mode == "remote" {
				kind = "git"
			}
			if mode == "relative-root" {
				root = "relative"
			}
			registration, _ := json.Marshal(root)
			config := filepath.Join(target.ConfigDir, "config.toml")
			files[config] = append(files[config], []byte("\n[marketplaces.style]\nsource_type=\""+kind+"\"\nsource="+string(registration)+"\n")...)
			plugin := map[string]any{"name": m.Identity.Name, "source": map[string]any{"source": "local", "path": target.Dir}}
			catalog := map[string]any{"name": "style", "plugins": []any{plugin}}
			switch mode {
			case "wrong-source":
				plugin["source"].(map[string]any)["path"] = t.TempDir()
			case "remote-plugin":
				plugin["source"].(map[string]any)["source"] = "git"
			case "wrong-marketplace":
				catalog["name"] = "other"
			case "duplicate":
				catalog["plugins"] = []any{plugin, plugin}
			case "missing-plugin":
				catalog["plugins"] = []any{}
			}
			catalogPath := filepath.Join(marketplace, ".agents", "plugins", "marketplace.json")
			files[catalogPath], _ = json.Marshal(catalog)
			if mode == "missing-catalog" {
				delete(files, catalogPath)
			}
			if mode == "bad-catalog" {
				files[catalogPath] = []byte("{")
			}
			cache := filepath.Join(target.ConfigDir, "plugins", "cache", "style", m.Identity.Name, m.Identity.Version)
			for _, item := range []struct{ label, path string }{{"hooks", "hooks/hooks.json"}, {"launcher", "bin/style"}, {"descriptor", ".codex-plugin/plugin.json"}} {
				path := filepath.Join(cache, filepath.FromSlash(item.path))
				files[path] = append([]byte(nil), files[filepath.Join(target.Dir, filepath.FromSlash(item.path))]...)
				if mode == "missing-"+item.label {
					delete(files, path)
				}
				if mode == "stale-"+item.label {
					files[path] = append(files[path], '\n')
				}
			}
			if mode == "unsafe-version" {
				m.Identity.Version = "../escape"
				rendered, err := Render(m, target)
				if err != nil {
					t.Fatal(err)
				}
				files[filepath.Join(target.Dir, ".codex-plugin", "plugin.json")], _ = fs.ReadFile(rendered, ".codex-plugin/plugin.json")
			}
			status, err := InspectRuntime(m, target, a2ConfigReader(files))
			if mode == "valid" {
				if err != nil || status.Root != cache || len(status.Events) != 1 {
					t.Fatalf("verified cache: %+v %v", status, err)
				}
			} else if err == nil || status.Root != "" {
				t.Fatalf("invalid cache accepted: %+v %v", status, err)
			}
		})
	}
}

func TestM3A2CachePathComponents(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "C:drive", "nul\x00byte"} {
		if probeCacheComponent(value) {
			t.Errorf("unsafe cache component accepted: %q", value)
		}
	}
	for _, value := range []string{"style-runtime", "style", "0.1.0", "0.1.0-preview.1"} {
		if !probeCacheComponent(value) {
			t.Errorf("valid cache component rejected: %q", value)
		}
	}
}
