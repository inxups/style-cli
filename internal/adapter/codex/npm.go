package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/inxups/style-cli/internal/saferead"
)

const npmPackage = "@openai/codex"

// npmTargets mirrors PLATFORM_PACKAGE_BY_TARGET in the package's
// bin/codex.js, keyed by GOOS/GOARCH.
var npmTargets = map[string]struct{ triple, pkg string }{
	"linux/amd64":   {"x86_64-unknown-linux-musl", "@openai/codex-linux-x64"},
	"linux/arm64":   {"aarch64-unknown-linux-musl", "@openai/codex-linux-arm64"},
	"darwin/amd64":  {"x86_64-apple-darwin", "@openai/codex-darwin-x64"},
	"darwin/arm64":  {"aarch64-apple-darwin", "@openai/codex-darwin-arm64"},
	"windows/amd64": {"x86_64-pc-windows-msvc", "@openai/codex-win32-x64"},
	"windows/arm64": {"aarch64-pc-windows-msvc", "@openai/codex-win32-arm64"},
}

// resolveNPMLauncher returns path unchanged unless it is the npm package's
// bin/codex.js, in which case it returns the platform binary that launcher
// spawns, located the way the launcher's require.resolve would find it.
func resolveNPMLauncher(path string) (string, error) {
	return resolveNPMLauncherFor(path, runtime.GOOS, runtime.GOARCH)
}

func resolveNPMLauncherFor(path, goos, goarch string) (string, error) {
	launcher, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, nil
	}
	bin := filepath.Dir(launcher)
	if filepath.Base(launcher) != "codex.js" || filepath.Base(bin) != "bin" {
		return path, nil
	}
	root := filepath.Dir(bin)
	data, err := saferead.ReadFile(filepath.Join(root, "package.json"), 1<<20)
	if err != nil {
		return path, nil
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Name != npmPackage {
		return path, nil
	}
	target, ok := npmTargets[goos+"/"+goarch]
	if !ok {
		return "", fmt.Errorf("codex npm launcher: unsupported platform %s/%s", goos, goarch)
	}
	name := "codex"
	if goos == "windows" {
		name = "codex.exe"
	}
	binary := filepath.Join(target.triple, "bin", name)
	// Node resolves the platform package from the launcher's directory
	// upwards, skipping directories that are themselves node_modules.
	for dir := bin; ; dir = filepath.Dir(dir) {
		if filepath.Base(dir) != "node_modules" {
			candidate := filepath.Join(dir, "node_modules", filepath.FromSlash(target.pkg))
			if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
				return executableFile(filepath.Join(candidate, "vendor", binary))
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return executableFile(filepath.Join(root, "vendor", binary))
}

func executableFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("codex npm launcher: native binary unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("codex npm launcher: native binary is not a regular file")
	}
	return path, nil
}
