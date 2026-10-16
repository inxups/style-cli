package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const syncManifest = "spec: style/v0.3\nname: editable\nversion: 0.1.0\ndescription: editable\nsettings:\n  - name: voice\n    default: direct\nrules:\n  - id: base\n    target: system\n    text: 'Use {{settings.voice}} original static.'\n  - from: rules/*.md\n"
const syncRule = "---\ntarget: tool\ntool: shell\nat: before\n---\nOriginal dynamic instruction.\n"

func syncFixture(t *testing.T, link bool) (*Service, string, string, string) {
	t.Helper()
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
	source := writeInstallPackage(t, filepath.Join(root, "source"), syncManifest)
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), syncRule)
	service := testCodexInstallService(t, config, filepath.Join(home, "adapters", "codex", "plugin"))
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{"codex"}, Link: link, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if !link {
		source = filepath.Join(home, "packages", "editable")
	}
	return service, home, source, config
}

func writeSyncFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSyncFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func installedTree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				result[path] = string(readSyncFile(t, path))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestSyncBlocksOrdinaryEditsAndTrustPreviewsThenCommits(t *testing.T) {
	service, home, source, config := syncFixture(t, false)
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Approved"))
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(syncManifest, "original static", "approved static"))
	before := installedTree(t, home, config)
	result, err := service.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || !reflect.DeepEqual(result.NeedsTrust, []string{"editable"}) || !reflect.DeepEqual(result.Changes[0].Files, []string{"STYLE.yaml", "rules/shell.md"}) {
		t.Fatalf("sync = %+v, %v", result, err)
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("blocked sync wrote files")
	}
	preview, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable"})
	if err != nil || !preview.AwaitingConfirmation || !preview.Changes[0].BaselineAvailable {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	encoded, _ := json.Marshal(preview.Changes[0])
	for _, want := range []string{"-Original dynamic instruction.", "+Approved dynamic instruction.", "Use direct original static.", "Use direct approved static."} {
		if !bytes.Contains(encoded, []byte(want)) {
			t.Fatalf("preview missing %q: %s", want, encoded)
		}
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("preview wrote files")
	}
	result, err = service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true})
	if err != nil || !result.Changed || result.AwaitingConfirmation {
		t.Fatalf("trust = %+v, %v", result, err)
	}
	index := readSyncFile(t, filepath.Join(home, "build", "index.json"))
	if !bytes.Contains(index, []byte("Approved dynamic instruction.")) {
		t.Fatalf("old index: %s", index)
	}
	if !bytes.Contains(readSyncFile(t, filepath.Join(config, "config.toml")), []byte("Use direct approved static.")) {
		t.Fatal("static output not updated")
	}
	lock, err := loadLock(home)
	if err != nil || len(filesChanged(lock.Packages[0], mustInstalledSnapshot(t, home, lock.Packages[0]))) > 0 {
		t.Fatalf("lock not updated: %+v, %v", lock, err)
	}
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed || len(result.NeedsTrust) > 0 {
		t.Fatalf("idempotent sync = %+v, %v", result, err)
	}
	if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "editable", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(approvalPath(home, "editable")); !os.IsNotExist(err) {
		t.Fatalf("approval survived uninstall: %v", err)
	}
}

func mustInstalledSnapshot(t *testing.T, home string, item lockPackage) map[string][]byte {
	t.Helper()
	files, err := installedSnapshot(home, item)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestTrustCompilesExactlyTheConfirmedSnapshot(t *testing.T) {
	service, home, source, _ := syncFixture(t, false)
	path := filepath.Join(source, "rules", "shell.md")
	approved := strings.ReplaceAll(syncRule, "Original", "Approved")
	writeSyncFile(t, path, approved)
	result, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Confirm: func(preview SyncResult) (bool, error) {
		if len(preview.Changes[0].SourceDiff) != 1 || !strings.Contains(preview.Changes[0].SourceDiff[0].Diff, "+Approved dynamic") {
			t.Fatalf("unexpected preview: %+v", preview)
		}
		writeSyncFile(t, path, "malformed later source")
		preview.Changes[0].SourceDiff[0].Diff = "callback mutation"
		return true, nil
	}})
	if err != nil || !result.Changed {
		t.Fatalf("trust = %+v, %v", result, err)
	}
	lock, _ := loadLock(home)
	if lock.Packages[0].Files["rules/shell.md"] != hashBytes([]byte(approved)) {
		t.Fatal("lock hashed reread source")
	}
	if !bytes.Contains(readSyncFile(t, filepath.Join(home, "build", "index.json")), []byte("Approved dynamic instruction.")) {
		t.Fatal("index compiled reread source")
	}
	baseline, _, err := loadApproval(home, lock.Packages[0])
	if err != nil || string(baseline.Files["rules/shell.md"]) != approved {
		t.Fatalf("baseline not approved snapshot: %+v, %v", baseline, err)
	}
	if strings.Contains(result.Changes[0].SourceDiff[0].Diff, "callback mutation") {
		t.Fatal("callback mutated transaction summary")
	}
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(result.NeedsTrust) != 1 {
		t.Fatalf("later edit was not detected: %+v, %v", result, err)
	}
}

func TestLinkRequiresFirstTrustAndThenSyncsChanges(t *testing.T) {
	service, home, source, _ := syncFixture(t, true)
	before := installedTree(t, home)
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(result.NeedsTrust) != 1 {
		t.Fatalf("untrusted unchanged link = %+v, %v", result, err)
	}
	if !reflect.DeepEqual(before, installedTree(t, home)) {
		t.Fatal("untrusted link sync mutated installation")
	}
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true}); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Development"))
	result, err := service.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || !result.Changed || len(result.NeedsTrust) != 0 || len(result.Changes) != 1 {
		t.Fatalf("trusted link sync = %+v, %v", result, err)
	}
	if !bytes.Contains(readSyncFile(t, filepath.Join(home, "build", "index.json")), []byte("Development dynamic instruction.")) {
		t.Fatal("link source did not compile")
	}
	lock, _ := loadLock(home)
	if !lock.Packages[0].LinkTrusted || lock.Packages[0].Seq != 1 {
		t.Fatalf("lock = %+v", lock.Packages[0])
	}
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed {
		t.Fatalf("link sync not idempotent: %+v, %v", result, err)
	}
}

func TestLinkDriftIsVisibleBeforeFirstTrustAndAfterApproval(t *testing.T) {
	service, home, source, config := syncFixture(t, true)
	ctx := context.Background()
	// An unchanged development link still needs its one-time approval. The
	// empty file diff in this case does not indicate an undetected source edit.
	initial, err := service.Sync(ctx, SyncRequest{Home: home})
	if err != nil || len(initial.Changes) != 1 || len(initial.Changes[0].Files) != 0 || len(initial.NeedsTrust) != 1 {
		t.Fatalf("unchanged untrusted link = %+v, %v", initial, err)
	}
	manifest := filepath.Join(source, "STYLE.yaml")
	writeSyncFile(t, manifest, strings.ReplaceAll(syncManifest, "original static", "changed static"))
	before := installedTree(t, home, config)
	listed, err := service.List(ctx, ListRequest{Home: home})
	if err != nil || listed.Packages[0].HashMatches || !reflect.DeepEqual(listed.Packages[0].ChangedFiles, []string{"STYLE.yaml"}) {
		t.Fatalf("untrusted link drift = %+v, %v", listed, err)
	}
	blocked, err := service.Sync(ctx, SyncRequest{Home: home})
	if err != nil || len(blocked.Changes) != 1 || !reflect.DeepEqual(blocked.Changes[0].Files, []string{"STYLE.yaml"}) || len(blocked.NeedsTrust) != 1 {
		t.Fatalf("changed untrusted link = %+v, %v", blocked, err)
	}
	preview, err := service.Trust(ctx, TrustRequest{Home: home, Name: "editable"})
	if err != nil || len(preview.Changes) != 1 || len(preview.Changes[0].SourceDiff) != 1 || !strings.Contains(preview.Changes[0].SourceDiff[0].Diff, "+    text: 'Use {{settings.voice}} changed static.'") || len(preview.Changes[0].RenderedDiff) == 0 {
		t.Fatalf("link preview = %+v, %v", preview, err)
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("blocked sync or trust preview wrote files")
	}
	if _, err := service.Trust(ctx, TrustRequest{Home: home, Name: "editable", Yes: true}); err != nil {
		t.Fatal(err)
	}
	listed, err = service.List(ctx, ListRequest{Home: home})
	if err != nil || !listed.Packages[0].HashMatches {
		t.Fatalf("approved link = %+v, %v", listed, err)
	}
	writeSyncFile(t, manifest, strings.ReplaceAll(syncManifest, "original static", "next static"))
	listed, err = service.List(ctx, ListRequest{Home: home})
	if err != nil || listed.Packages[0].HashMatches {
		t.Fatalf("trusted link drift = %+v, %v", listed, err)
	}
	synced, err := service.Sync(ctx, SyncRequest{Home: home})
	if err != nil || !synced.Changed || len(synced.NeedsTrust) != 0 || !bytes.Contains(readSyncFile(t, filepath.Join(config, "config.toml")), []byte("next static")) {
		t.Fatalf("trusted link auto sync = %+v, %v", synced, err)
	}
}

func TestTrustWithoutLegacyBaselineDisclosesUnavailableOldBytes(t *testing.T) {
	service, home, source, _ := syncFixture(t, false)
	lock, err := loadLock(home)
	if err != nil {
		t.Fatal(err)
	}
	lock.Packages[0].Approval = ""
	if err := writeLock(home, lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(approvalPath(home, "editable")); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "New"))
	result, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true})
	if err != nil || result.Changes[0].BaselineAvailable {
		t.Fatalf("legacy trust = %+v, %v", result, err)
	}
	for _, diff := range result.Changes[0].SourceDiff {
		if strings.Contains(diff.Diff, "-Original dynamic instruction.") {
			t.Fatal("invented an unavailable previous body")
		}
	}
	lock, _ = loadLock(home)
	if _, available, err := loadApproval(home, lock.Packages[0]); err != nil || !available {
		t.Fatalf("new baseline = %t, %v", available, err)
	}
}
