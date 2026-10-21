package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func writeNPMFixture(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func TestResolveNPMLauncher(t *testing.T) {
	const vendor = "vendor/aarch64-apple-darwin/bin/codex"
	for _, mode := range []string{"nested", "hoisted", "bundled", "missing", "foreign-package", "native", "absent"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			pkg := filepath.Join(root, "lib", "node_modules", "@openai", "codex")
			launcher := filepath.Join(pkg, "bin", "codex.js")
			name := `{"name":"@openai/codex"}`
			if mode == "foreign-package" {
				name = `{"name":"not-codex"}`
			}
			writeNPMFixture(t, filepath.Join(pkg, "package.json"), name, 0o600)
			writeNPMFixture(t, launcher, "#!/usr/bin/env node\n", 0o755)
			// npm links bin/codex to the launcher.
			link := filepath.Join(root, "bin", "codex")
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(launcher, link); err != nil {
				t.Fatal(err)
			}
			var want string
			switch mode {
			case "nested", "foreign-package":
				platform := filepath.Join(pkg, "node_modules", "@openai", "codex-darwin-arm64")
				writeNPMFixture(t, filepath.Join(platform, "package.json"), `{}`, 0o600)
				want = filepath.Join(platform, filepath.FromSlash(vendor))
				writeNPMFixture(t, want, "native", 0o755)
			case "hoisted":
				platform := filepath.Join(root, "lib", "node_modules", "@openai", "codex-darwin-arm64")
				writeNPMFixture(t, filepath.Join(platform, "package.json"), `{}`, 0o600)
				want = filepath.Join(platform, filepath.FromSlash(vendor))
				writeNPMFixture(t, want, "native", 0o755)
			case "bundled":
				want = filepath.Join(pkg, filepath.FromSlash(vendor))
				writeNPMFixture(t, want, "native", 0o755)
			case "missing":
				platform := filepath.Join(pkg, "node_modules", "@openai", "codex-darwin-arm64")
				writeNPMFixture(t, filepath.Join(platform, "package.json"), `{}`, 0o600)
			case "native":
				link = filepath.Join(root, "native", "codex")
				writeNPMFixture(t, link, "native", 0o755)
			case "absent":
				link = filepath.Join(root, "absent", "codex")
			}
			got, err := resolveNPMLauncherFor(link, "darwin", "arm64")
			switch mode {
			case "missing":
				if err == nil {
					t.Fatalf("resolved %q without the platform binary", got)
				}
			case "foreign-package", "native", "absent":
				if err != nil || got != link {
					t.Fatalf("non-launcher path changed: %q %v", got, err)
				}
			default:
				canonical, canonicalErr := filepath.EvalSymlinks(want)
				if canonicalErr != nil {
					t.Fatal(canonicalErr)
				}
				if err != nil || got != canonical {
					t.Fatalf("resolved %q %v, want %q", got, err, canonical)
				}
			}
		})
	}
	if _, err := resolveNPMLauncherFor(filepath.Join(t.TempDir(), "codex"), "plan9", "amd64"); err != nil {
		t.Fatalf("non-launcher path must not depend on platform support: %v", err)
	}
}
