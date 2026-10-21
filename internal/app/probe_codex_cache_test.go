package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/probe"
	"github.com/speakeasy-api/agenthooks"
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

// An npm install puts a Node launcher on PATH that spawns the platform
// binary; hooks see that binary, never the launcher, as their harness.
func TestCodexProbeBindsNativeBinaryBehindNPMLauncher(t *testing.T) {
	requirePOSIXFixture(t, "POSIX npm launcher layout")
	platform, ok := map[string]string{"darwin/arm64": "codex-darwin-arm64/vendor/aarch64-apple-darwin", "darwin/amd64": "codex-darwin-x64/vendor/x86_64-apple-darwin", "linux/arm64": "codex-linux-arm64/vendor/aarch64-unknown-linux-musl", "linux/amd64": "codex-linux-x64/vendor/x86_64-unknown-linux-musl"}[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		t.Skip("no Codex npm platform package for this host")
	}
	for _, mode := range []string{"native", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f := a2Install(t, "codex-user")
			pkg := filepath.Join(f.root, "npm", "lib", "node_modules", "@openai", "codex")
			launcher := filepath.Join(pkg, "bin", "codex.js")
			native := filepath.Join(pkg, "node_modules", "@openai", filepath.FromSlash(platform), "bin", "codex")
			for path, data := range map[string]string{
				filepath.Join(pkg, "package.json"): `{"name":"@openai/codex"}`,
				launcher:                           "#!/usr/bin/env node\n",
				filepath.Join(pkg, "node_modules", "@openai", filepath.Dir(filepath.Dir(filepath.FromSlash(platform))), "package.json"): `{}`,
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "native" {
				if err := os.MkdirAll(filepath.Dir(native), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(native, []byte("native codex platform binary"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			f.service.probeDeps.lookPath = func(name string) (string, error) {
				if name == "codex" {
					return launcher, nil
				}
				return f.runtime, nil
			}
			// The hook's harness ancestor is the platform binary the
			// launcher spawned.
			lookup := f.service.probeDeps.lookup
			f.service.probeDeps.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
				if pid != 400 || mode != "native" {
					return lookup(pid)
				}
				id, err := agenthooks.ExecutableIdentity(native)
				return agenthooks.ProcessIdentity{PID: pid, ParentPID: 1, Birth: "harness-birth", Executable: native, ExecutableID: id}, err
			}
			result, err := f.service.Probe(context.Background(), ProbeRequest{Home: f.home, Timeout: time.Second, Ready: func(ProbeReady) error {
				f.emit(t, "codex", nil)
				return nil
			}})
			if mode == "missing" {
				if err == nil || !a2HasCode(result, "origin-unverified") || result.Targets[0].Status != "unavailable" {
					t.Fatalf("launcher without its binary was bound: %+v %v", result, err)
				}
				return
			}
			if err != nil || !result.Targets[0].CallbackObserved || result.Targets[0].HarnessSHA256 != probe.Hash(readSyncFile(t, native)) {
				t.Fatalf("native binary behind launcher not bound: %+v %v", result, err)
			}
			if result.Targets[0].HarnessSHA256 == probe.Hash(readSyncFile(t, launcher)) {
				t.Fatal("bound the Node launcher instead of the native binary")
			}
			a2NoLeases(t, f.home)
		})
	}
}
