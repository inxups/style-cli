package app

import (
	"bytes"
	"context"
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
			// STYLE renders the catalog beside the plugin directory.
			catalog := filepath.Join(f.root, ".agents", "plugins", "marketplace.json")
			readSyncFile(t, catalog)
			cache := codexPluginCache(f.codexConfig)
			artifacts := map[string]string{"hooks": "hooks/hooks.json", "launcher": "bin/style", "descriptor": ".codex-plugin/plugin.json"}
			result, err := f.service.Probe(context.Background(), ProbeRequest{Home: f.home, Timeout: time.Second, Ready: func(ProbeReady) error {
				f.emit(t, "codex", func(receipt *probe.Receipt) {
					if mode == "source-root" {
						canonical, canonicalErr := filepath.EvalSymlinks(f.plugin)
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

func TestCodexPluginProbeRequiresRegistration(t *testing.T) {
	enabled := []byte("[plugins.\"style-runtime@style\"]\nenabled = true\n")
	for mode, replacement := range map[string][]byte{
		"unregistered": nil,
		"disabled":     []byte("[plugins.\"style-runtime@style\"]\nenabled = false\n"),
		"stale-cache":  enabled,
	} {
		t.Run(mode, func(t *testing.T) {
			f := a2Install(t, "codex-plugin")
			path := filepath.Join(f.codexConfig, "config.toml")
			data := readSyncFile(t, path)
			if !bytes.Contains(data, enabled) {
				t.Fatalf("fixture lacks plugin enablement: %s", data)
			}
			if err := os.WriteFile(path, bytes.Replace(data, enabled, replacement, 1), 0o600); err != nil {
				t.Fatal(err)
			}
			if mode == "stale-cache" {
				hooks := filepath.Join(codexPluginCache(f.codexConfig), "hooks", "hooks.json")
				if err := os.WriteFile(hooks, append(readSyncFile(t, hooks), ' '), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := f.service.Probe(context.Background(), ProbeRequest{Home: f.home, Timeout: time.Second, Ready: func(ProbeReady) error {
				t.Error("probe became ready without a registered, enabled and cached plugin")
				return nil
			}})
			if err == nil || !a2HasCode(result, "probe-installation-unavailable") || result.Targets[0].Status != "unavailable" {
				t.Fatalf("%s plugin probed: %+v %v", mode, result, err)
			}
			a2NoLeases(t, f.home)
		})
	}
}
