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
)

func versionTestService(t *testing.T, root, codexVersion, claudeVersion string) *Service {
	t.Helper()
	codexInstall := adapter.Installation{Provider: "codex", Version: codexVersion, ConfigDir: filepath.Join(root, "codex")}
	claudeInstall := adapter.Installation{Provider: "claude-code", Version: claudeVersion, ConfigDir: filepath.Join(root, "claude")}
	codexDef := codex.DefinitionForInstallation(codexInstall)
	codexDef.Detector = fixedDetector{codexInstall}
	codexDef.Routes = []adapter.Route{codex.NewUserRouteForCommand(codexInstall.ConfigDir, "/tmp/style", nil)}
	claudeDef := claude.DefinitionForInstallation(claudeInstall)
	claudeDef.Detector = fixedDetector{claudeInstall}
	service, err := New(codexDef, claudeDef)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestExampleRequirementsSkipOldAndUnknownHarnessesWithoutMutation(t *testing.T) {
	for _, version := range []string{"old", "unknown"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			codexVersion, claudeVersion := "0.159.2", "2.1.299"
			if version == "unknown" {
				codexVersion, claudeVersion = "", ""
			}
			service := versionTestService(t, root, codexVersion, claudeVersion)
			paths := []string{filepath.Join(root, "codex", "config.toml"), filepath.Join(root, "claude", "settings.json")}
			for _, path := range paths {
				writeSyncFile(t, path, "# external bytes\n")
			}
			home := filepath.Join(root, "style")
			t.Setenv("STYLE_HOME", home)
			var stdout, stderr bytes.Buffer
			code := RunWithServiceInput(context.Background(), service, repositoryRoot(t), []string{"install", "testdata/packages/terse-reviewer", "--harness", "codex,claude-code"}, approve(), &stdout, &stderr)
			if code != ExitOK || !strings.Contains(stdout.String(), "skipped terse-reviewer") || strings.Count(stdout.String(), "warning harness-version:") != 2 {
				t.Fatalf("install exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(home); !os.IsNotExist(err) {
				t.Fatalf("skipped install created STYLE_HOME: %v", err)
			}
			for _, path := range paths {
				if got := string(readSyncFile(t, path)); got != "# external bytes\n" {
					t.Fatalf("skipped install changed %s: %q", path, got)
				}
			}
			planned, err := service.Plan(context.Background(), PlanRequest{Source: filepath.Join(repositoryRoot(t), "testdata", "packages", "terse-reviewer"), Harnesses: []string{"codex,claude-code"}})
			if err != nil || len(planned.Reports) != 0 || len(planned.Diagnostics) != 2 {
				t.Fatalf("plan=%+v err=%v", planned, err)
			}
		})
	}
}

func TestExampleRequirementsInstallOnlyCompatibleHarnessAndRebuildPerPackage(t *testing.T) {
	root := t.TempDir()
	service := versionTestService(t, root, "0.159.2", "2.1.300")
	home := filepath.Join(root, "style")
	// An older package still works on Codex and must keep its route when a
	// second package excludes that harness.
	old := writeInstallPackage(t, filepath.Join(root, "old"), "spec: style/v0.3\nname: old\nversion: 0.1.0\ndescription: old\nrules:\n  - id: old\n    target: system\n    text: Keep this old package.\n")
	if _, err := service.Install(context.Background(), InstallRequest{Source: old, Home: home, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	before := readSyncFile(t, filepath.Join(root, "codex", "config.toml"))
	result, err := service.Install(context.Background(), InstallRequest{Source: filepath.Join(repositoryRoot(t), "testdata", "packages", "terse-reviewer"), Home: home, Harnesses: []string{"codex,claude-code"}, Yes: true})
	if err != nil || !result.Changed || len(result.Reports) != 1 || result.Reports[0].Provider != "claude-code" {
		t.Fatalf("install=%+v err=%v", result, err)
	}
	if !bytes.Equal(before, readSyncFile(t, filepath.Join(root, "codex", "config.toml"))) {
		t.Fatal("incompatible package changed the existing Codex configuration")
	}
	var index struct {
		Static    map[string][]json.RawMessage `json:"static"`
		Harnesses map[string]json.RawMessage   `json:"harnesses"`
	}
	if err := json.Unmarshal(readSyncFile(t, filepath.Join(home, "build", "index.json")), &index); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(index.Harnesses["codex"], []byte("terse-reviewer/")) {
		t.Fatal("incompatible package compiled dynamic Codex rules")
	}
	for _, entry := range index.Static["codex"] {
		if bytes.Contains(entry, []byte("terse-reviewer/")) {
			t.Fatal("incompatible package compiled static Codex rules")
		}
	}
	if synced, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || synced.Changed {
		t.Fatalf("sync=%+v err=%v", synced, err)
	}
	if !bytes.Equal(before, readSyncFile(t, filepath.Join(root, "codex", "config.toml"))) {
		t.Fatal("sync reintroduced the incompatible package")
	}
}
