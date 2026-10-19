package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestRunSyncAndTrustExitCodesAndDiffOutput(t *testing.T) {
	service, home, source, config := syncFixture(t, false)
	t.Setenv("STYLE_HOME", home)
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Reviewed"))
	var stdout, stderr bytes.Buffer
	var input io.Reader
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return RunWithServiceInput(context.Background(), service, source, args, input, &stdout, &stderr)
	}
	if code := run("sync", "--json"); code != ExitSyncChanged {
		t.Fatalf("sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result SyncResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !reflect.DeepEqual(result.NeedsTrust, []string{"editable"}) {
		t.Fatalf("sync JSON = %s, %v", stdout.String(), err)
	}
	before := installedTree(t, home, config)
	if code := run("trust", "editable"); code != ExitInvalid || !strings.Contains(stdout.String(), "Source diff:") || !strings.Contains(stdout.String(), "Rendered diff") || !strings.Contains(stderr.String(), "trust canceled") {
		t.Fatalf("preview code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("CLI preview mutated files")
	}
	input = approve()
	if code := run("trust", "editable", "--json"); code != ExitOK {
		t.Fatalf("trust code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !result.Changed || result.ApprovalTarget != "editable" || result.Changes[0].NeedsTrust {
		t.Fatalf("trust JSON = %s, %v", stdout.String(), err)
	}
	if code := run("sync"); code != ExitOK || !strings.Contains(stdout.String(), "changed=false") {
		t.Fatalf("idempotent sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, args := range [][]string{{"sync", "extra"}, {"sync", "--yes"}, {"trust", "editable", "--yes"}, {"trust"}, {"trust", "editable", "extra"}, {"trust", "editable", "--unknown"}, {"trust", "missing"}} {
		if code := run(args...); code != ExitInvalid {
			t.Fatalf("%v code=%d", args, code)
		}
	}
	for _, command := range []string{"sync", "trust"} {
		if code := run(command, "--help"); code != 0 || !strings.HasPrefix(stdout.String(), "style "+command) {
			t.Fatalf("%s help code=%d stdout=%s", command, code, stdout.String())
		}
	}
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Huge"))
	definition := service.definitions[adapter.ProviderID("codex")]
	definition.Limits.Input = tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}
	service.definitions[adapter.ProviderID("codex")] = definition
	before = installedTree(t, home, config)
	input = approve()
	if code := run("trust", "editable"); code != ExitBlocked || !strings.Contains(stderr.String(), "T003") {
		t.Fatalf("blocked trust code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("blocked CLI trust mutated files")
	}
}

func TestSyncSingleFilePreservesSourceFilenameAndSequence(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(map[bool]string{false: "managed", true: "link"}[link], func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "style")
			source := filepath.Join(root, "different.style.md")
			data := "---\nspec: style/v0.3\nname: single\nversion: 0.1.0\ndescription: single\ntarget: system\n---\nOriginal single instruction.\n"
			writeSyncFile(t, source, data)
			service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
			if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Link: link, Yes: true}); err != nil {
				t.Fatal(err)
			}
			if listed, err := service.List(context.Background(), ListRequest{Home: home}); err != nil || !listed.Packages[0].HashMatches {
				t.Fatalf("list = %+v, %v", listed, err)
			}
			if link {
				if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "single", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			if !link {
				source = filepath.Join(home, "packages", "single.style.md")
			}
			writeSyncFile(t, source, strings.ReplaceAll(data, "Original", "Changed"))
			if link {
				if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || !result.Changed {
					t.Fatalf("single link = %+v, %v", result, err)
				}
			} else {
				if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(result.NeedsTrust) != 1 {
					t.Fatalf("single sync = %+v, %v", result, err)
				}
				if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "single", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			lock, _ := loadLock(home)
			if lock.Packages[0].Files["different.style.md"] == "" || lock.Packages[0].Seq != 1 {
				t.Fatalf("source identity lost: %+v", lock)
			}
			if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed {
				t.Fatalf("single sync = %+v, %v", result, err)
			}
		})
	}
}

func TestSyncStateMaintenanceAndChangedAssetDiffs(t *testing.T) {
	service, home, source, _ := syncFixture(t, false)
	old := filepath.Join(home, "state", "old")
	recent := filepath.Join(home, "state", "recent")
	for _, dir := range []string{old, recent} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ago := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, ago, ago); err != nil {
		t.Fatal(err)
	}
	result, err := service.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || result.ExpiredSessions != 1 {
		t.Fatalf("maintenance = %+v, %v", result, err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(source, "empty.txt"), "")
	writeSyncFile(t, filepath.Join(source, "binary.dat"), string([]byte{0xff, 0, 1}))
	result, err = service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true})
	if err != nil || len(result.Changes[0].SourceDiff) != 2 {
		t.Fatalf("assets = %+v, %v", result, err)
	}
	if !strings.Contains(result.Changes[0].SourceDiff[0].Diff, "Binary") || !strings.Contains(result.Changes[0].SourceDiff[1].Diff, "Empty file") {
		t.Fatalf("asset diffs = %+v", result.Changes[0].SourceDiff)
	}
	if err := os.Remove(filepath.Join(source, "empty.txt")); err != nil {
		t.Fatal(err)
	}
	result, err = service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true})
	if err != nil || !strings.Contains(result.Changes[0].SourceDiff[0].Diff, "present=true -> present=false") {
		t.Fatalf("removed file = %+v, %v", result, err)
	}
}
