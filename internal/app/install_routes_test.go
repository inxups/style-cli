package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"runtime"
)

func TestRunInstallSwitchesRoutesWithoutResidueAndKeepsExternalConfig(t *testing.T) {
	root := t.TempDir()
	home, configDir := filepath.Join(root, "style"), filepath.Join(root, "codex")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	external := []byte("model = 'external'\n\n[features]\nexternal = true")
	if err := os.WriteFile(configPath, external, 0o640); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(configDir, "hooks.json")
	otherHooks := []byte(`{"outside":"keep","hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"/tmp/other --owner=other"}]}]}}`)
	if err := os.WriteFile(hooksPath, otherHooks, 0o600); err != nil {
		t.Fatal(err)
	}
	installation := adapter.Installation{Provider: "codex", ConfigDir: configDir, Version: "0.159.3"}
	definition := codex.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STYLE_HOME", home)
	source := writeInstallPackage(t, filepath.Join(root, "source"), "spec: style/v0.3\nname: route-test\nversion: 0.1.0\ndescription: test\nrules:\n  - id: base\n    target: system\n    text: Keep responses concise.\n")
	previousRoute := ""
	for _, route := range fixtureRoutes(t, []string{"codex-plugin", "codex-user", "codex-plugin", "codex-user"}) {
		var stdout, stderr bytes.Buffer
		args := []string{"install", source, "--harness=codex", "--route=codex=" + route, "--yes", "--json"}
		code := RunWithService(context.Background(), service, root, args, &stdout, &stderr)
		var result InstallResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 0 || result.Changed != (route != previousRoute) {
			t.Fatalf("%s exit=%d err=%v stdout=%s stderr=%s", route, code, err, stdout.String(), stderr.String())
		}
		lock, err := loadLock(home)
		if err != nil || len(lock.Packages) != 1 || lock.Packages[0].Seq != 1 || lock.Harnesses["codex"].Route != route {
			t.Fatalf("lock=%+v err=%v", lock, err)
		}
		config, err := os.ReadFile(configPath)
		if err != nil || !bytes.Contains(config, []byte("Keep responses concise.")) || !bytes.Contains(config, []byte("external = true")) || bytes.Contains(config, []byte("style-runtime@style")) != (route == "codex-plugin") {
			t.Fatalf("%s configuration=%s err=%v", route, config, err)
		}
		pluginFile := filepath.Join(home, "adapters", "codex", "plugin", ".codex-plugin", "plugin.json")
		_, pluginErr := os.Stat(pluginFile)
		if route == "codex-plugin" && pluginErr != nil || route == "codex-user" && !os.IsNotExist(pluginErr) {
			t.Fatalf("%s plugin residue: %v", route, pluginErr)
		}
		hooks, _ := os.ReadFile(hooksPath)
		if !bytes.Contains(hooks, []byte("--owner=other")) || bytes.Contains(hooks, []byte("agenthooks")) != (route == "codex-user") && runtime.GOOS != "windows" {
			t.Fatalf("%s hook owner residue: %s", route, hooks)
		}
		if runtime.GOOS == "windows" && strings.Contains(string(hooks), "EncodedCommand") != (route == "codex-user") {
			t.Fatal("Windows runtime owner residue", string(hooks))
		}
		stdout.Reset()
		if code := RunWithService(context.Background(), service, root, args, &stdout, &stderr); code != 0 {
			t.Fatalf("repeat %s: exit=%d %s", route, code, stderr.String())
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Changed {
			t.Fatalf("repeat %s changed: %s %v", route, stdout.String(), err)
		}
		previousRoute = route
	}
	if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(configPath)
	if !bytes.Equal(config, external) {
		t.Fatalf("external bytes not restored: %q want %q", config, external)
	}
	hooks, _ := os.ReadFile(hooksPath)
	if !bytes.Equal(hooks, otherHooks) {
		t.Fatalf("external JSON bytes not restored: %q want %q", hooks, otherHooks)
	}
	var outside map[string]any
	if json.Unmarshal(hooks, &outside) != nil || outside["outside"] != "keep" || bytes.Contains(hooks, []byte("--owner=style")) || !bytes.Contains(hooks, []byte("--owner=other")) {
		t.Fatalf("external hook configuration=%s", hooks)
	}
}

func TestReinstallRejectsUnknownRouteAndDetectsStoredDrift(t *testing.T) {
	service, home := inspectionFixture(t)
	source := filepath.Join(repositoryRoot(t), "testdata", "packages", "terse-reviewer")
	before, _ := os.ReadFile(filepath.Join(home, "style.lock.yaml"))
	_, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Yes: true, Routes: map[string]string{"codex": "missing"}})
	if err == nil || !strings.Contains(err.Error(), "unknown route") {
		t.Fatalf("unknown route = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "packages", "terse-reviewer", "extra.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = service.Install(context.Background(), InstallRequest{Source: source, Home: home, Yes: true, Routes: map[string]string{"codex": "codex-user"}})
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stored drift = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(home, "style.lock.yaml"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed reconfiguration changed lock")
	}
}

func TestInstallNewPackageCanSwitchExistingRoute(t *testing.T) {
	service, home := inspectionFixture(t)
	source := writeInstallPackage(t, filepath.Join(t.TempDir(), "source"), "spec: style/v0.3\nname: second\nversion: 0.1.0\ndescription: test\nrules:\n  - id: base\n    target: system\n    text: Additional instruction.\n")
	result, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Yes: true, Routes: map[string]string{"codex": "codex-user"}})
	if err != nil || !result.Changed {
		t.Fatalf("switch during new install: %+v %v", result, err)
	}
	lock, err := loadLock(home)
	if err != nil || len(lock.Packages) != 2 || lock.Packages[0].Seq != 1 || lock.Packages[1].Seq != 2 || lock.Harnesses["codex"].Route != "codex-user" {
		t.Fatalf("lock=%+v err=%v", lock, err)
	}
	if _, err := os.Stat(filepath.Join(home, "adapters", "codex", "plugin", ".codex-plugin", "plugin.json")); !os.IsNotExist(err) {
		t.Fatalf("old route remains: %v", err)
	}
}

func TestInstallRetainsExistingFailClosedPolicy(t *testing.T) {
	root := t.TempDir()
	installation := adapter.Installation{Provider: "claude-code", Version: "2.1.295", ConfigDir: filepath.Join(root, "claude")}
	definition := claude.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "style")
	for i, name := range []string{"first", "second"} {
		source := writeInstallPackage(t, filepath.Join(root, name), "spec: style/v0.3\nname: "+name+"\nversion: 0.1.0\ndescription: test\nrules:\n  - id: guard\n    target: tool\n    tool: shell\n    at: before\n    text: Be careful.\n")
		request := InstallRequest{Source: source, Home: home, Yes: true, Harnesses: []string{"claude-code"}}
		if i == 0 {
			request.FailClosed = []string{"claude-code"}
		}
		if _, err := service.Install(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	lock, err := loadLock(home)
	if err != nil || !lock.Harnesses["claude-code"].FailClosed {
		t.Fatalf("lock=%+v err=%v", lock, err)
	}
	data, err := os.ReadFile(filepath.Join(installation.ConfigDir, "settings.json"))
	if err != nil || !bytes.Contains(data, []byte(`"onFailure": "block"`)) {
		t.Fatalf("policy lost: %s %v", data, err)
	}
}
