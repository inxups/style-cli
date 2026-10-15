package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

type fixedDetector struct {
	installation adapter.Installation
}

func (detector fixedDetector) Detect(context.Context) (adapter.Installation, bool, error) {
	return detector.installation, true, nil
}

type staleChangesetRoute struct {
	root string
	path string
}

func (route staleChangesetRoute) Name() string { return "test-route" }

func (route staleChangesetRoute) Available(context.Context, adapter.Installation) (bool, string, error) {
	return true, "available", nil
}

func (staleChangesetRoute) Trust() adapter.TrustInfo {
	return adapter.TrustInfo{Kind: adapter.TrustPreseed}
}

func (route staleChangesetRoute) Plan(context.Context, []adapter.HookSpec, []adapter.StaticBlock) (adapter.Changeset, error) {
	return adapter.Changeset{Files: []adapter.FileChange{{Path: route.path, Root: route.root, Mode: 0o600, Before: nil, After: []byte("STYLE")}}}, nil
}

func (staleChangesetRoute) Apply(context.Context, adapter.Changeset) error { return nil }

func (staleChangesetRoute) Remove(context.Context, adapter.Owner) (adapter.Changeset, error) {
	return adapter.Changeset{}, nil
}

func (route staleChangesetRoute) Status(context.Context, adapter.Installation) (adapter.RouteStatus, error) {
	if err := os.WriteFile(route.path, []byte("external update"), 0o600); err != nil {
		return adapter.RouteStatus{}, err
	}
	return adapter.RouteStatus{Available: true}, nil
}

func testCodexInstallService(t *testing.T, configDir, pluginDir string) *Service {
	t.Helper()
	installation := adapter.Installation{Provider: agenthooks.ProviderCodex, Version: "0.159.3", ConfigDir: configDir}
	facts := codex.DefaultFacts()
	definition := codex.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation: installation}
	definition.Facts = facts
	definition.StaticPositions = facts
	definition.Routes = []adapter.Route{
		codex.NewPluginRouteForCommand(pluginDir, configDir, "/tmp/style", &facts),
		codex.NewUserRouteForCommand(configDir, "/tmp/style", &facts),
	}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestInstallWritesPackageLockIndexAndProviderRoute(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	configDir := filepath.Join(root, "codex")
	pluginDir := filepath.Join(styleHome, "adapters", "codex", "plugin")
	service := testCodexInstallService(t, configDir, pluginDir)
	result, err := service.Install(context.Background(), InstallRequest{
		Source: filepath.Join(repositoryRoot(t), "example", "terse-reviewer"), Home: styleHome,
		Harnesses: []string{"codex"}, Yes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Package != "terse-reviewer" || len(result.Routes) != 1 || result.Routes[0] != "codex=codex-plugin" {
		t.Fatalf("install result = %#v", result)
	}
	for _, path := range []string{
		filepath.Join(styleHome, "style.lock.yaml"),
		filepath.Join(styleHome, "build", "index.json"),
		filepath.Join(styleHome, "packages", "terse-reviewer", "STYLE.yaml"),
		filepath.Join(pluginDir, ".codex-plugin", "plugin.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected install output %s: %v", path, err)
		}
	}
	lock, err := os.ReadFile(filepath.Join(styleHome, "style.lock.yaml"))
	if err != nil || !strings.Contains(string(lock), "name: terse-reviewer") || !strings.Contains(string(lock), "route: codex-plugin") {
		t.Fatalf("lock = %q, err=%v", lock, err)
	}
	second, err := service.Install(context.Background(), InstallRequest{
		Source: filepath.Join(repositoryRoot(t), "example", "terse-reviewer"), Home: styleHome,
		Harnesses: []string{"codex"}, Yes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatalf("repeated install changed state: %#v", second)
	}
}

func TestInstallRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	_, err := service.Install(context.Background(), InstallRequest{Source: filepath.Join(repositoryRoot(t), "example", "terse-reviewer"), Home: filepath.Join(root, "style")})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("install without confirmation error = %v, want UsageError", err)
	}
}

func writeInstallPackage(t *testing.T, root, manifest string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInstallSettingsArePersisted(t *testing.T) {
	root := t.TempDir()
	packageDir := writeInstallPackage(t, filepath.Join(root, "configured"), "spec: style/v0.3\nname: configured\nversion: 0.1.0\ndescription: configured\nsettings:\n  - name: voice\n    description: output voice\nrules:\n  - id: base\n    target: system\n    text: 'Use {{settings.voice}} voice.'\n")
	styleHome := filepath.Join(root, "style")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	result, err := service.Install(context.Background(), InstallRequest{
		Source: packageDir, Home: styleHome, Harnesses: []string{"codex"}, Settings: map[string]string{"voice": "direct"}, Yes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Replace {
		t.Fatalf("install result = %#v", result)
	}
	settingsPath := filepath.Join(styleHome, "settings", "configured.yaml")
	settings, err := os.ReadFile(settingsPath)
	if err != nil || !bytes.Contains(settings, []byte("voice: direct")) {
		t.Fatalf("settings = %q, err=%v", settings, err)
	}
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %v, want 0600", info.Mode().Perm())
	}
	lock, err := os.ReadFile(filepath.Join(styleHome, "style.lock.yaml"))
	if err != nil || !bytes.Contains(lock, []byte("- voice")) {
		t.Fatalf("lock settings = %q, err=%v", lock, err)
	}
}

func TestInstallRejectsMissingSettingAndRuntimeRequirementWithoutMutation(t *testing.T) {
	root := t.TempDir()
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	missing := writeInstallPackage(t, filepath.Join(root, "missing"), "spec: style/v0.3\nname: missing\nversion: 0.1.0\ndescription: missing\nsettings:\n  - name: voice\nrules:\n  - id: base\n    target: system\n    text: body\n")
	styleHome := filepath.Join(root, "style")
	_, err := service.Install(context.Background(), InstallRequest{Source: missing, Home: styleHome, Harnesses: []string{"codex"}, Yes: true})
	var usage *UsageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "voice") {
		t.Fatalf("missing setting error = %v", err)
	}
	if _, statErr := os.Stat(styleHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing setting mutated home: %v", statErr)
	}

	incompatible := writeInstallPackage(t, filepath.Join(root, "incompatible"), "spec: style/v0.3\nname: incompatible\nversion: 0.1.0\ndescription: incompatible\nrequires:\n  runtime: '>=9.0.0'\nrules:\n  - id: base\n    target: system\n    text: body\n")
	_, err = service.Install(context.Background(), InstallRequest{Source: incompatible, Home: filepath.Join(root, "style-2"), Harnesses: []string{"codex"}, Yes: true})
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "requires.runtime") {
		t.Fatalf("runtime requirement error = %v", err)
	}
}

func TestInstallRequiresReplaceConfirmationAndCommandYes(t *testing.T) {
	root := t.TempDir()
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	replace := writeInstallPackage(t, filepath.Join(root, "replace"), "spec: style/v0.3\nname: replace\nversion: 0.1.0\ndescription: replace\nrules:\n  - id: base\n    target: system\n    mode: replace\n    text: body\n")
	styleHome := filepath.Join(root, "style")
	_, err := service.Install(context.Background(), InstallRequest{Source: replace, Home: styleHome, Harnesses: []string{"codex"}, Yes: true})
	var usage *UsageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "allow-replace") {
		t.Fatalf("replace error = %v", err)
	}
	if _, statErr := os.Stat(styleHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("replace rejection mutated home: %v", statErr)
	}
	t.Setenv("STYLE_HOME", styleHome)
	var stdout, stderr bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"install", replace, "--harness", "codex", "--yes"}, &stdout, &stderr); code != ExitInvalid || stderr.Len() == 0 {
		t.Fatalf("replace install without --allow-replace code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, statErr := os.Stat(styleHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("replace rejection mutated home: %v", statErr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := RunWithService(context.Background(), service, root, []string{"install", replace, "--harness", "codex"}, &stdout, &stderr); code != ExitInvalid || stderr.Len() == 0 {
		t.Fatalf("install without --yes code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestInstallT003BlocksBeforeWritingAnyState(t *testing.T) {
	root := t.TempDir()
	packageDir := writeInstallPackage(t, filepath.Join(root, "oversized"), "spec: style/v0.3\nname: oversized\nversion: 0.1.0\ndescription: oversized\nrules:\n  - id: shell\n    target: tool\n    tool: shell\n    text: a dynamically injected instruction\n")
	styleHome := filepath.Join(root, "style")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	definition := service.definitions[adapter.ProviderID("codex")]
	definition.Limits.Input = tokens.InputLimit{Value: 1, Unit: tokens.TokensUnit}
	service.definitions[adapter.ProviderID("codex")] = definition
	_, err := service.Install(context.Background(), InstallRequest{Source: packageDir, Home: styleHome, Harnesses: []string{"codex"}, Yes: true})
	var blocked *BlockedError
	if !errors.As(err, &blocked) || !strings.Contains(err.Error(), "T003") {
		t.Fatalf("oversized install error = %v, want T003 BlockedError", err)
	}
	if _, statErr := os.Stat(styleHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("T003 rejection mutated STYLE_HOME: %v", statErr)
	}
}

func TestInstallRollsBackPackageWhenRouteSnapshotTurnsStale(t *testing.T) {
	root := t.TempDir()
	packageDir := writeInstallPackage(t, filepath.Join(root, "rollback"), "spec: style/v0.3\nname: rollback\nversion: 0.1.0\ndescription: rollback\nrules:\n  - id: base\n    target: system\n    text: body\n")
	styleHome := filepath.Join(root, "style")
	providerRoot := filepath.Join(root, "provider")
	if err := os.MkdirAll(providerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providerRoot, "config.json")
	installation := adapter.Installation{Provider: agenthooks.ProviderCodex, Version: "test", ConfigDir: providerRoot}
	definition := codex.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation: installation}
	definition.Routes = []adapter.Route{staleChangesetRoute{root: providerRoot, path: providerPath}}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Install(context.Background(), InstallRequest{Source: packageDir, Home: styleHome, Harnesses: []string{"codex"}, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "planned file changed before apply") {
		t.Fatalf("install error = %v, want stale route changeset", err)
	}
	if _, statErr := os.Stat(filepath.Join(styleHome, "packages", "rollback")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("package was not rolled back: %v", statErr)
	}
	for _, path := range []string{filepath.Join(styleHome, "style.lock.yaml"), filepath.Join(styleHome, "build", "index.json")} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("failed install left state %s: %v", path, statErr)
		}
	}
	data, err := os.ReadFile(providerPath)
	if err != nil || string(data) != "external update" {
		t.Fatalf("concurrent external config = %q, err=%v", data, err)
	}
}

func TestRunInstallPrintsCapabilityAndCostReport(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	t.Setenv("STYLE_HOME", styleHome)
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	var stdout, stderr bytes.Buffer
	code := RunWithService(context.Background(), service, repositoryRoot(t), []string{"install", "example/terse-reviewer", "--harness", "codex", "--yes"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("install exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, marker := range []string{"Harness: codex", "Token (estimated", "native input limit:", "session cost", "installed terse-reviewer"} {
		if !strings.Contains(stdout.String(), marker) {
			t.Errorf("install output missing %q: %s", marker, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "Review each changed line") {
		t.Fatal("install report leaked rule body")
	}
}
