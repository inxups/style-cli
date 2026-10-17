package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/probe"
)

func TestM3A2CodexNativeCacheBinding(t *testing.T) {
	for _, mode := range []string{"valid", "source-root", "hooks", "launcher", "descriptor", "marketplace", "registration"} {
		t.Run(mode, func(t *testing.T) {
			f := a2Install(t, "codex-plugin")
			config := filepath.Join(f.codexConfig, "config.toml")
			root, _ := json.Marshal(f.root)
			if err := os.WriteFile(config, append(readSyncFile(t, config), []byte("\n[marketplaces.style]\nsource_type=\"local\"\nsource="+string(root)+"\n")...), 0o600); err != nil {
				t.Fatal(err)
			}
			catalog := filepath.Join(f.root, ".agents", "plugins", "marketplace.json")
			if err := os.MkdirAll(filepath.Dir(catalog), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(catalog, []byte(`{"name":"style","plugins":[{"name":"style-runtime","source":{"source":"local","path":"./plugin"}}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			cache := filepath.Join(f.codexConfig, "plugins", "cache", "style", "style-runtime", "0.1.0")
			artifacts := map[string]string{"hooks": "hooks/hooks.json", "launcher": "bin/style", "descriptor": ".codex-plugin/plugin.json"}
			for _, name := range artifacts {
				path := filepath.Join(cache, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, readSyncFile(t, filepath.Join(f.plugin, filepath.FromSlash(name))), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			result, err := f.service.Probe(context.Background(), ProbeRequest{Home: f.home, Timeout: time.Second, Ready: func(ProbeReady) error {
				f.emit(t, "codex", func(receipt *probe.Receipt) {
					if mode != "source-root" {
						canonical, canonicalErr := filepath.EvalSymlinks(cache)
						if canonicalErr != nil {
							t.Fatal(canonicalErr)
						}
						receipt.PluginRootSHA256 = probe.HashString(canonical)
					}
				})
				path := ""
				if name, ok := artifacts[mode]; ok {
					path = filepath.Join(cache, filepath.FromSlash(name))
				} else if mode == "marketplace" {
					path = catalog
				} else if mode == "registration" {
					path = config
				}
				if path != "" {
					return os.WriteFile(path, append(readSyncFile(t, path), ' '), 0o600)
				}
				return nil
			}})
			switch mode {
			case "valid":
				if err != nil || !result.Targets[0].CallbackObserved {
					t.Fatalf("cache receipt rejected: %+v %v", result, err)
				}
			case "source-root":
				if err == nil || result.Targets[0].CallbackObserved || result.Status != "timed-out" {
					t.Fatalf("source root accepted for registered cache: %+v %v", result, err)
				}
			default:
				if err == nil || !a2HasCode(result, "installation-changed") || result.Status != "error" {
					t.Fatalf("cache drift accepted: %+v %v", result, err)
				}
			}
			if !result.Cleanup.Complete {
				t.Fatal("cache probe cleanup incomplete")
			}
			a2NoLeases(t, f.home)
		})
	}
}
