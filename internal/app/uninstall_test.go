package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesOnlyStyleOwnerAndRestoresLastRoute(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	t.Setenv("STYLE_HOME", styleHome)
	configDir := filepath.Join(root, "codex")
	pluginDir := filepath.Join(styleHome, "adapters", "codex", "plugin")
	service := testCodexInstallService(t, configDir, pluginDir)
	configPath := filepath.Join(configDir, "config.toml")
	beforeExternal := []byte("# external\n")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, beforeExternal, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), InstallRequest{Source: filepath.Join(repositoryRoot(t), "example", "terse-reviewer"), Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	var output, errOutput bytes.Buffer
	code := RunWithService(context.Background(), service, root, []string{"uninstall", "terse-reviewer", "--yes"}, &output, &errOutput)
	if code != ExitOK {
		t.Fatalf("uninstall exit=%d stdout=%s stderr=%s", code, output.String(), errOutput.String())
	}
	if !strings.Contains(output.String(), "uninstalled terse-reviewer") {
		t.Fatalf("uninstall output=%q", output.String())
	}
	for _, path := range []string{
		filepath.Join(styleHome, "style.lock.yaml"),
		filepath.Join(styleHome, "build", "index.json"),
		filepath.Join(styleHome, "packages", "terse-reviewer"),
		filepath.Join(pluginDir, ".codex-plugin", "plugin.json"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("uninstall left %s: %v", path, err)
		}
	}
	if got, err := os.ReadFile(configPath); err != nil || !bytes.Equal(got, beforeExternal) {
		t.Fatalf("external config changed: %q, err=%v", got, err)
	}
}

func TestUninstallRequiresConfirmationAndKeepsRemainingPackage(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	first := filepath.Join(repositoryRoot(t), "example", "terse-reviewer")
	second := writeInstallPackage(t, filepath.Join(root, "second"), "spec: style/v0.3\nname: second\nversion: 0.1.0\ndescription: second\nrules:\n  - id: base\n    target: system\n    text: second\n")
	if _, err := service.Install(context.Background(), InstallRequest{Source: first, Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), InstallRequest{Source: second, Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Uninstall(context.Background(), UninstallRequest{Name: "terse-reviewer", Home: styleHome}); err == nil {
		t.Fatal("uninstall without confirmation succeeded")
	}
	result, err := service.Uninstall(context.Background(), UninstallRequest{Name: "terse-reviewer", Home: styleHome, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Remaining) != 1 || result.Remaining[0] != "second" {
		t.Fatalf("remaining = %#v", result.Remaining)
	}
	if _, err := os.Stat(filepath.Join(styleHome, "packages", "second", "STYLE.yaml")); err != nil {
		t.Fatalf("remaining package removed: %v", err)
	}
	lock, err := os.ReadFile(filepath.Join(styleHome, "style.lock.yaml"))
	if err != nil || !bytes.Contains(lock, []byte("name: second")) || bytes.Contains(lock, []byte("name: terse-reviewer")) || !bytes.Contains(lock, []byte("config_dir:")) {
		t.Fatalf("lock after partial uninstall=%q, err=%v", lock, err)
	}
}

func TestRunUninstallAllPreservesLinkedSource(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	t.Setenv("STYLE_HOME", styleHome)
	source := writeInstallPackage(t, filepath.Join(root, "linked-source"), "spec: style/v0.3\nname: linked\nversion: 0.1.0\ndescription: linked\nrules:\n  - id: base\n    target: system\n    text: linked\n")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(styleHome, "plugin"))
	if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: styleHome, Harnesses: []string{"codex"}, Link: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	var output, errOutput bytes.Buffer
	code := RunWithService(context.Background(), service, root, []string{"uninstall", "--all", "--yes", "--json"}, &output, &errOutput)
	if code != ExitOK {
		t.Fatalf("uninstall --all exit=%d stdout=%s stderr=%s", code, output.String(), errOutput.String())
	}
	var report struct {
		Removed []string `json:"removed"`
		Changed bool     `json:"changed"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil || len(report.Removed) != 1 || report.Removed[0] != "linked" || !report.Changed {
		t.Fatalf("uninstall --all JSON = %q, decoded=%#v, err=%v", output.String(), report, err)
	}
	if _, err := os.Stat(filepath.Join(source, "STYLE.yaml")); err != nil {
		t.Fatalf("uninstall removed linked source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(styleHome, "style.lock.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uninstall --all left lock file: %v", err)
	}
}
