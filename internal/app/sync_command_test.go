package app

import (
	"bytes"
	"context"
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

func TestRunSyncApprovalExitCodesAndDiffOutput(t *testing.T) {
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
	before := installedTree(t, home, config)
	// Without an answer the approval is declined: exit 1, nothing written.
	if code := run("sync", "--json"); code != ExitInvalid || !strings.Contains(stderr.String(), "Source diff:") || !strings.Contains(stderr.String(), "Approve editable? [y/N]") {
		t.Fatalf("sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result SyncResult
	if err := decodeReport(stdout.Bytes(), &result); err != nil || !result.AwaitingConfirmation || len(result.Approved) != 0 || !strings.Contains(stdout.String(), `"confirmation-declined"`) {
		t.Fatalf("sync JSON = %s, %v", stdout.String(), err)
	}
	if code := run("sync", "editable"); code != ExitInvalid || !strings.Contains(stdout.String(), "Source diff:") || !strings.Contains(stdout.String(), "Rendered diff") || !strings.Contains(stderr.String(), "style sync: confirmation-declined: sync canceled; nothing was changed") {
		t.Fatalf("preview code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("CLI preview mutated files")
	}
	input = approve()
	if code := run("sync", "--json"); code != ExitOK {
		t.Fatalf("sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := decodeReport(stdout.Bytes(), &result); err != nil || !result.Changed || !reflect.DeepEqual(result.Approved, []string{"editable"}) || len(result.NeedsTrust) != 0 || result.Changes[0].NeedsTrust {
		t.Fatalf("sync JSON = %s, %v", stdout.String(), err)
	}
	// An approved package has nothing to confirm.
	input = nil
	if code := run("sync", "editable"); code != ExitOK || strings.Contains(stderr.String(), "[y/N]") {
		t.Fatalf("unchanged sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if code := run("sync"); code != ExitOK || !strings.Contains(stdout.String(), "changed=false") {
		t.Fatalf("idempotent sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, args := range [][]string{{"sync", "editable", "extra"}, {"sync", "--yes"}, {"sync", "editable", "--unknown"}, {"trust", "editable"}} {
		if code := run(args...); code != ExitUsage {
			t.Fatalf("%v code=%d", args, code)
		}
	}
	if code := run("sync", "missing"); code != ExitInvalid || !strings.Contains(stderr.String(), "request-rejected") {
		t.Fatalf("missing code=%d stderr=%s", code, stderr.String())
	}
	if code := run("sync", "--help"); code != 0 || !strings.HasPrefix(stdout.String(), "style sync [name]") {
		t.Fatalf("help code=%d stdout=%s", code, stdout.String())
	}
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Huge"))
	definition := service.definitions[adapter.ProviderID("codex")]
	definition.Limits.Input = tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}
	service.definitions[adapter.ProviderID("codex")] = definition
	before = installedTree(t, home, config)
	input = approve()
	if code := run("sync", "editable"); code != ExitBlocked || !strings.Contains(stderr.String(), "T003") {
		t.Fatalf("blocked sync code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("blocked CLI sync mutated files")
	}
}

// A bare sync asks for each package that needs approval, one answer line
// each; a later decline keeps the approvals already applied.
func TestRunSyncApprovesPackagesOneByOne(t *testing.T) {
	for _, test := range []struct {
		answers  string
		exit     int
		approved []string
	}{
		{"y\ny\n", ExitOK, []string{"editable", "second"}},
		{"y\nn\n", ExitInvalid, []string{"editable"}},
	} {
		t.Run(strings.TrimSpace(strings.ReplaceAll(test.answers, "\n", " ")), func(t *testing.T) {
			service, home, source, _ := syncFixture(t, false)
			t.Setenv("STYLE_HOME", home)
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "second"), a4Manifest("second", "Second body."))
			if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: second, Harnesses: []string{"codex"}, Yes: true}); err != nil {
				t.Fatal(err)
			}
			writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Reviewed"))
			managed := filepath.Join(home, "packages", "second", "STYLE.yaml")
			writeSyncFile(t, managed, strings.ReplaceAll(string(readSyncFile(t, managed)), "Second body.", "Second edited."))
			var stdout, stderr bytes.Buffer
			code := RunWithServiceInput(context.Background(), service, source, []string{"sync", "--json"}, strings.NewReader(test.answers), &stdout, &stderr)
			var result SyncResult
			if err := decodeReport(stdout.Bytes(), &result); code != test.exit || err != nil || !reflect.DeepEqual(result.Approved, test.approved) {
				t.Fatalf("code=%d result=%+v err=%v\nstderr=%s", code, result, err, stderr.String())
			}
			if strings.Count(stderr.String(), "[y/N]") != 2 {
				t.Fatalf("prompts: %s", stderr.String())
			}
			if test.exit != ExitOK && !strings.Contains(stdout.String(), "sync canceled; already approved and applied: editable") {
				t.Fatalf("decline message: %s", stdout.String())
			}
			pending, err := service.Sync(context.Background(), SyncRequest{Home: home})
			if want := len(test.approved) < 2; err != nil || (len(pending.NeedsTrust) > 0) != want {
				t.Fatalf("after sync: %+v, %v", pending, err)
			}
		})
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
