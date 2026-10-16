package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
)

func TestTrustRebuildsClaudeSurfaceAndRetainsExternalConfigAndFailClosed(t *testing.T) {
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "claude")
	writeSyncFile(t, filepath.Join(config, "settings.json"), `{"external":"keep","hooks":{"Stop":[{"hooks":[{"type":"command","command":"outside"}]}]}}`)
	writeSyncFile(t, filepath.Join(config, "CLAUDE.md"), "External instructions.\n")
	installation := adapter.Installation{Provider: "claude-code", Version: "2.1.295", ConfigDir: config}
	definition := claude.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	source := writeInstallPackage(t, filepath.Join(root, "source"), syncManifest)
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), syncRule)
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Yes: true, FailClosed: []string{"claude-code"}}); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(home, "packages", "editable")
	writeSyncFile(t, filepath.Join(managed, "rules", "shell.md"), strings.ReplaceAll(syncRule, "tool: shell", "tool: file.edit"))
	writeSyncFile(t, filepath.Join(managed, "STYLE.yaml"), strings.ReplaceAll(syncManifest, "original static", "updated static"))
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true}); err != nil {
		t.Fatal(err)
	}
	settings := readSyncFile(t, filepath.Join(config, "settings.json"))
	for _, want := range []string{`"external": "keep"`, `"command": "outside"`, `"onFailure": "block"`, `"matcher": "^(Edit)$"`} {
		if !bytes.Contains(settings, []byte(want)) {
			t.Fatalf("missing %s in %s", want, settings)
		}
	}
	if bytes.Contains(settings, []byte(`"matcher": "^(Bash)$"`)) {
		t.Fatalf("stale Claude surface: %s", settings)
	}
	static := readSyncFile(t, filepath.Join(config, "rules", "style-editable.md"))
	if string(readSyncFile(t, filepath.Join(config, "CLAUDE.md"))) != "External instructions.\n" || !bytes.Contains(static, []byte("Use direct updated static.")) || bytes.Contains(static, []byte("original static")) {
		t.Fatalf("Claude static = %s", static)
	}
	lock, _ := loadLock(home)
	if !lock.Harnesses["claude-code"].FailClosed || lock.Harnesses["claude-code"].ConfigDir != config {
		t.Fatalf("installation facts lost: %+v", lock)
	}
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed {
		t.Fatalf("Claude sync not idempotent: %+v, %v", result, err)
	}
}
