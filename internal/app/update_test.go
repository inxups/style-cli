package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testgit"
)

var a4Routes = []string{"codex-user", "codex-plugin", "claude-user"}

func a4Manifest(name, body string) string {
	return "spec: style/v0.3\nname: " + name + "\nversion: 1.0.0\ndescription: update fixture\nrules:\n" +
		"  - {id: dynamic, target: tool, tool: shell, at: before, text: '" + body + "'}\n" +
		"  - {id: static, target: system, text: 'Static " + body + "'}\n"
}

func a4Setup(t *testing.T, route, manifest string, link bool) (*Service, string, string, string) {
	t.Helper()
	s, home, config, sourcePath, _ := nativeInstallFixture(t, route)
	writeSyncFile(t, filepath.Join(sourcePath, "STYLE.yaml"), manifest)
	a4Install(t, s, home, route, sourcePath, link)
	return s, home, config, sourcePath
}

func a4Install(t *testing.T, s *Service, home, route, sourcePath string, link bool) {
	t.Helper()
	request := InstallRequest{Home: home, Source: sourcePath, Link: link, Yes: true,
		Harnesses: []string{nativeProvider(route)}, Routes: map[string]string{nativeProvider(route): route}}
	if _, err := s.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

type a4FileState struct {
	Mode os.FileMode
	Time int64
	Data string
}

// Cache publication is deliberately outside the installation transaction.
// Include runtime state/stats/probe files to detect unsolicited maintenance.
func a4State(t *testing.T, home, config string) map[string]a4FileState {
	t.Helper()
	result := map[string]a4FileState{}
	for _, root := range []string{home, config} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if p == filepath.Join(home, "cache") {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			info, err := os.Lstat(p)
			if err != nil {
				return err
			}
			var data []byte
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(p)
				if err != nil {
					return err
				}
				data = []byte("symlink:" + target)
			} else {
				data, err = os.ReadFile(p)
				if err != nil {
					return err
				}
			}
			result[p] = a4FileState{info.Mode(), info.ModTime().UnixNano(), string(data)}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func a4AssertState(t *testing.T, before map[string]a4FileState, home, config string) {
	t.Helper()
	after := a4State(t, home, config)
	if !reflect.DeepEqual(before, after) {
		for path, old := range before {
			if current, ok := after[path]; !ok || old != current {
				t.Errorf("installation changed: %s\nbefore=%#v\nafter=%#v", path, old, current)
			}
		}
		for path := range after {
			if _, ok := before[path]; !ok {
				t.Errorf("unexpected installed file: %s", path)
			}
		}
		t.FailNow()
	}
}

func a4ExpectError(t *testing.T, result UpdateResult, err error, code string) {
	t.Helper()
	if err == nil || result.Status != "error" || result.Changed || result.AwaitingConfirmation {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if code != "" && !slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool { return d.Code == code }) {
		t.Fatalf("missing %s: %+v (%v)", code, result.Diagnostics, err)
	}
}

func TestM3A4SelectionAndCLIOptions(t *testing.T) {
	s := testService(t)
	t.Setenv("STYLE_HOME", filepath.Join(t.TempDir(), "empty"))
	invalid := [][]string{{}, {"first", "--all"}, {"first", "second"}, {"--all", "--ref=refs/heads/main"},
		{"first", "--ref"}, {"first", "--ref="}, {"first", "--ref=x", "--ref=y"}, {"first", "--yes"},
		{"--all", "--all"}, {"first", "--set=a=b"}, {"first", "--harness=codex"},
		{"first", "--route=codex=codex-user"}, {"first", "--path=pkg"}, {"first", "--source=elsewhere"},
		{"first", "--link"}, {"first", "--frozen"}, {"first", "--fail-closed=claude-code"}, {"first", "--json", "--json"}}
	for _, args := range invalid {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			exit := RunWithService(context.Background(), s, t.TempDir(), append([]string{"update"}, args...), &out, &errOut)
			if exit != ExitUsage || out.Len() != 0 || errOut.Len() == 0 {
				t.Fatalf("%d stdout=%s stderr=%s", exit, &out, &errOut)
			}
		})
	}
	for _, args := range [][]string{{"update", "--help"}, {"--help"}} {
		var out bytes.Buffer
		if RunWithService(context.Background(), s, t.TempDir(), args, &out, &bytes.Buffer{}) != ExitOK || !strings.Contains(out.String(), updateCommand.summary()) {
			t.Fatalf("missing help: %s", &out)
		}
	}
	for _, request := range []UpdateRequest{{}, {Name: "first", All: true}, {All: true, Ref: "refs/heads/main"}, {Name: "first", Ref: "main"}} {
		result, err := s.Update(context.Background(), request)
		a4ExpectError(t, result, err, "source-invalid")
	}
	result, err := s.Update(context.Background(), UpdateRequest{All: true})
	a4ExpectError(t, result, err, "request-rejected")
	if _, err := os.Stat(os.Getenv("STYLE_HOME")); !os.IsNotExist(err) {
		t.Fatal("empty update created an installation")
	}
}

func TestM3A4PreviewNoopAndApply(t *testing.T) {
	for _, route := range a4Routes {
		t.Run(route, func(t *testing.T) {
			s, home, config, origin := a4Setup(t, route, a4Manifest("first", "Before."), false)
			ctx := context.Background()
			for _, yes := range []bool{false, true} {
				before := a4State(t, home, config)
				inputs, err := captureManagement(home)
				if err != nil {
					t.Fatal(err)
				}
				called := false
				result, err := s.Update(ctx, UpdateRequest{Home: home, Name: "first", Yes: yes, Confirm: func(UpdateResult) (bool, error) {
					called = true
					return true, nil
				}})
				if err != nil || result.Status != "unchanged" || result.Changed || result.AwaitingConfirmation || called {
					t.Fatalf("no-op=%+v %v callback=%t", result, err, called)
				}
				if err := inputs.verify(); err != nil {
					t.Fatalf("no-op replaced installation identity: %v", err)
				}
				a4AssertState(t, before, home, config)
			}
			// Preserve semantically identical lock/settings formatting on no-op.
			lockPath := filepath.Join(home, "style.lock.yaml")
			writeSyncFile(t, lockPath, "# user backup comment\n"+string(readSyncFile(t, lockPath)))
			before := a4State(t, home, config)
			result, err := s.Update(ctx, UpdateRequest{Home: home, Name: "first"})
			if err != nil || result.Changed || result.AwaitingConfirmation {
				t.Fatalf("formatted no-op=%+v %v", result, err)
			}
			a4AssertState(t, before, home, config)
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), a4Manifest("first", "After."))
			before = a4State(t, home, config)
			preview, err := s.Update(ctx, UpdateRequest{Home: home, Name: "first"})
			if err != nil || preview.Status != "confirmation-required" || !preview.AwaitingConfirmation || preview.Changed || len(preview.Changes) != 1 || !preview.Changes[0].SourceChanged || !preview.Changes[0].RenderedChanged || len(preview.Reports) != 1 {
				t.Fatalf("preview=%+v %v", preview, err)
			}
			a4AssertState(t, before, home, config)
			applied, err := s.Update(ctx, UpdateRequest{Home: home, Name: "first", Yes: true})
			if err != nil || applied.Status != "applied" || !applied.Changed || applied.AwaitingConfirmation || applied.Changes[0].After.Version != "1.0.0" {
				t.Fatalf("apply=%+v %v", applied, err)
			}
			if !bytes.Contains(readSyncFile(t, filepath.Join(home, "packages", "first", "STYLE.yaml")), []byte("After.")) {
				t.Fatal("did not update from original source")
			}
			before = a4State(t, home, config)
			result, err = s.Update(ctx, UpdateRequest{Home: home, All: true})
			if err != nil || result.Status != "unchanged" {
				t.Fatalf("post-update no-op=%+v %v", result, err)
			}
			a4AssertState(t, before, home, config)
		})
	}
}

func TestM3A4GitRefUpdates(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		for _, selection := range []string{"branch", "tag", "oid", "change-ref", "revision-only", "downgrade", "missing-ref"} {
			t.Run(format+"/"+selection, func(t *testing.T) {
				s, home, config, _, _ := nativeInstallFixture(t, "codex-user")
				manifest := a4Manifest("first", "Old revision.")
				f := testgit.New(t, format, []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(manifest)}})
				s.sources = f.Resolver()
				initial := f.Commit
				request := a3Request(home, "codex-user")
				if selection == "tag" {
					f.Run(nil, "tag", "-a", "v1", "-m", "annotated", initial)
					request.Ref = "refs/tags/v1"
				} else if selection == "oid" {
					request.Ref = initial
				}
				if _, err := s.Install(context.Background(), request); err != nil {
					t.Fatal(err)
				}
				nextManifest := strings.ReplaceAll(manifest, "Old revision.", "Next revision.")
				if selection == "revision-only" || selection == "oid" {
					nextManifest = manifest
				} else if selection == "downgrade" {
					nextManifest = strings.ReplaceAll(nextManifest, "version: 1.0.0", "version: 0.5.0")
				}
				next := f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(nextManifest)}})
				update := UpdateRequest{Home: home, Name: "first", Yes: true}
				switch selection {
				case "tag":
					f.Run(nil, "tag", "-f", "-a", "v1", "-m", "moved", next)
				case "change-ref":
					f.Run(nil, "update-ref", "refs/heads/review", next)
					update.Ref = "refs/heads/review"
				case "missing-ref":
					f.Run(nil, "symbolic-ref", "HEAD", "refs/heads/elsewhere")
					f.Run(nil, "update-ref", "refs/heads/elsewhere", next)
					f.Run(nil, "update-ref", "-d", "refs/heads/main")
				}
				before := a4State(t, home, config)
				result, err := s.Update(context.Background(), update)
				if selection == "missing-ref" {
					a4ExpectError(t, result, err, "source-fetch-failed")
					if result.Changes[0].After != nil {
						t.Fatal("failed acquisition fabricated an after revision")
					}
					a4AssertState(t, before, home, config)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if selection == "oid" {
					if result.Status != "unchanged" || result.Changed || result.Changes[0].After.Source.Git.Commit != initial {
						t.Fatalf("pinned update=%+v", result)
					}
					a4AssertState(t, before, home, config)
					return
				}
				if !result.Changed || result.Status != "applied" || !result.Changes[0].SourceChanged || result.Changes[0].After.Source.Git.Commit != next {
					t.Fatalf("Git update=%+v", result)
				}
				if selection == "revision-only" && (len(result.Changes[0].SourceDiff) != 0 || result.Changes[0].RenderedChanged) {
					t.Fatalf("revision-only mixed source/rendered effects: %+v", result.Changes[0])
				}
				lock, err := loadLock(home)
				if err != nil || lock.Format != 2 || lock.Packages[0].Seq != 1 || lock.Packages[0].Source.Git.Commit != next {
					t.Fatalf("lock=%+v %v", lock, err)
				}
			})
		}
	}
}

func TestM3A4LocalSourceKindsAndDrift(t *testing.T) {
	for _, kind := range []string{"copy-dir", "copy-file", "link-dir", "link-file"} {
		for _, failure := range []string{"none", "missing-source", "type-change", "name-change", "managed-drift", "ref-option"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				s, home, config, origin, _ := nativeInstallFixture(t, "codex-user")
				link, single := strings.HasPrefix(kind, "link"), strings.HasSuffix(kind, "file")
				manifest := a4Manifest("first", "Old.")
				if single {
					origin = filepath.Join(filepath.Dir(origin), "original.style.md")
					manifest = "---\nspec: style/v0.3\nname: first\nversion: 1.0.0\ndescription: single\ntarget: system\n---\nOld.\n"
					writeSyncFile(t, origin, manifest)
				} else {
					writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), manifest)
				}
				a4Install(t, s, home, "codex-user", origin, link)
				sourceFile := origin
				if !single {
					sourceFile = filepath.Join(origin, "STYLE.yaml")
				}
				writeSyncFile(t, sourceFile, strings.ReplaceAll(manifest, "Old.", "New."))
				request := UpdateRequest{Home: home, Name: "first", Yes: true}
				switch failure {
				case "missing-source":
					if err := os.RemoveAll(origin); err != nil {
						t.Fatal(err)
					}
				case "type-change":
					if err := os.RemoveAll(origin); err != nil {
						t.Fatal(err)
					}
					if single {
						writeInstallPackage(t, origin, a4Manifest("first", "New."))
					} else {
						writeSyncFile(t, origin, "plain file")
					}
				case "name-change":
					writeSyncFile(t, sourceFile, strings.ReplaceAll(manifest, "name: first", "name: other"))
				case "managed-drift":
					if !link {
						managed := filepath.Join(home, "packages", "first", "STYLE.yaml")
						if single {
							managed = filepath.Join(home, "packages", "first.style.md")
						}
						writeSyncFile(t, managed, strings.ReplaceAll(manifest, "Old.", "Unapproved."))
					}
				case "ref-option":
					request.Ref = "refs/heads/main"
				}
				before := a4State(t, home, config)
				result, err := s.Update(context.Background(), request)
				if failure != "none" && !(failure == "managed-drift" && link) {
					a4ExpectError(t, result, err, "")
					a4AssertState(t, before, home, config)
					return
				}
				if err != nil || !result.Changed || !result.Changes[0].SourceChanged || result.Changes[0].After.Source.Kind != sourceKind(single) || result.Changes[0].After.Source.Path != result.Changes[0].Before.Source.Path || link && !result.Changes[0].After.LinkTrusted {
					t.Fatalf("source update=%+v %v", result, err)
				}
				if link {
					if _, err := os.Stat(filepath.Join(home, "packages", "first")); !os.IsNotExist(err) {
						t.Fatal("link copied into managed storage")
					}
				}
			})
		}
	}
}

func TestM3A4SettingsAndSequence(t *testing.T) {
	for _, change := range []string{"retain", "drop", "new-default", "new-default-shadow", "required", "required-shadow", "remove-all", "unknown-stored"} {
		t.Run(change, func(t *testing.T) {
			manifest := strings.Replace(a4Manifest("first", "Use {{settings.voice}}."), "rules:", "settings: [{name: voice, default: Original}]\nrules:", 1)
			s, home, config, origin := a4Setup(t, "codex-user", manifest, false)
			// Set an existing value through the private configuration and approve
			// its effect before beginning the update under test.
			writeSyncFile(t, filepath.Join(home, "settings", "first.yaml"), "# preserve comments\nvoice: Configured\n")
			if _, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Yes: true}); err != nil {
				t.Fatal(err)
			}
			lock, _ := loadLock(home)
			lock.Packages[0].Seq = 17
			if err := writeLock(home, lock); err != nil {
				t.Fatal(err)
			}
			// Refresh only this explicit selection to bind the new sequence.
			if _, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			next := strings.ReplaceAll(manifest, "default: Original", "default: Upstream")
			next = strings.ReplaceAll(next, "version: 1.0.0", "version: 0.9.0")
			switch change {
			case "drop", "remove-all":
				next = strings.ReplaceAll(next, "settings: [{name: voice, default: Upstream}]\n", "")
				next = strings.ReplaceAll(next, "{{settings.voice}}", "plain")
			case "new-default", "new-default-shadow":
				next = strings.ReplaceAll(next, "default: Upstream}]", "default: Upstream}, {name: added, default: NewDefault}]")
				if change == "new-default-shadow" {
					writeSyncFile(t, filepath.Join(home, "settings", "first.yaml"), "voice: Configured\nadded: UNDECLARED-SHADOW\n")
				}
			case "required", "required-shadow":
				next = strings.ReplaceAll(next, "default: Upstream}]", "default: Upstream}, {name: required}]")
				t.Setenv("STYLE_SETTING_REQUIRED", "must not be adopted")
				if change == "required-shadow" {
					writeSyncFile(t, filepath.Join(home, "settings", "first.yaml"), "voice: Configured\nrequired: UNDECLARED-SHADOW\n")
				}
			case "unknown-stored":
				writeSyncFile(t, filepath.Join(home, "settings", "first.yaml"), "voice: Configured\nunused: PRIVATE-UNUSED\n")
			}
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), next)
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
			if change == "required" || change == "required-shadow" {
				a4ExpectError(t, result, err, "source-invalid")
				a4AssertState(t, before, home, config)
				return
			}
			if err != nil || !result.Changed || result.Changes[0].Sequence != 17 {
				t.Fatalf("%+v %v", result, err)
			}
			values, err := readSettingValues(home, "first")
			if err != nil {
				t.Fatal(err)
			}
			if change == "drop" || change == "remove-all" {
				if len(values) != 0 || !slices.Contains(result.Changes[0].SettingsRemoved, "voice") {
					t.Fatalf("drop=%v %+v", values, result.Changes[0])
				}
			} else {
				if values["voice"] != "Configured" || values["unused"] != "" {
					t.Fatalf("settings=%v", values)
				}
				if strings.HasPrefix(change, "new-default") && (values["added"] != "NewDefault" || !slices.Contains(result.Changes[0].SettingsAdded, "added")) {
					t.Fatalf("new setting=%v", values)
				}
			}
			for _, d := range result.Diagnostics {
				if strings.Contains(d.Message, "Configured") || strings.Contains(d.Message, "PRIVATE-UNUSED") {
					t.Fatal("settings value entered diagnostic")
				}
			}
			lock, _ = loadLock(home)
			if lock.Packages[0].Seq != 17 || lock.Packages[0].Version != "0.9.0" || lock.Format != 1 {
				t.Fatalf("lock=%+v", lock)
			}
		})
	}
}

func TestM3A4UnselectedUsesOnlyApprovedBytes(t *testing.T) {
	for _, kind := range []string{"copy", "link", "trusted-link", "native-copy", "native-link"} {
		t.Run(kind, func(t *testing.T) {
			s, home, config, first := a4Setup(t, "codex-user", a4Manifest("first", "First original."), false)
			manifest := a4Manifest("second", "Second approved.")
			if strings.HasPrefix(kind, "native") {
				manifest = nativePackageYAML("second")
			}
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), manifest)
			link := strings.Contains(kind, "link")
			a4Install(t, s, home, "codex-user", second, link)
			if kind == "trusted-link" || kind == "native-link" {
				if _, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "second", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			// Both original and managed sources can contain edits. Neither is
			// consulted for an unselected package during update.
			dirty := strings.ReplaceAll(manifest, "Second approved.", "UNSELECTED-NEW-BODY")
			dirty = strings.ReplaceAll(dirty, "printf earlier", "printf UNSELECTED-NEW-COMMAND")
			writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), dirty)
			if !link {
				writeSyncFile(t, filepath.Join(home, "packages", "second", "STYLE.yaml"), dirty)
			}
			writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), a4Manifest("first", "First updated."))
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
			if err != nil || !result.Changed || !slices.Equal(result.Selected, []string{"first"}) {
				t.Fatalf("%+v %v", result, err)
			}
			after := a4State(t, home, config)
			for path, state := range before {
				if strings.Contains(path, "/second/") || strings.HasSuffix(path, "/second.json") || strings.HasSuffix(path, "/second.yaml") {
					if after[path] != state {
						t.Fatalf("updated unselected file %s", path)
					}
				}
			}
			for _, path := range []string{filepath.Join(home, "build", "index.json"), filepath.Join(home, "build", "explain.json"), filepath.Join(config, "hooks.json")} {
				data := readSyncFile(t, path)
				if bytes.Contains(data, []byte("UNSELECTED-NEW")) {
					t.Fatalf("unselected candidate entered %s", path)
				}
			}
		})
	}
}

func TestM3A4AuthenticatedBaselinesRequired(t *testing.T) {
	for _, damage := range []string{"missing", "corrupt", "no-digest", "wrong-files", "legacy-native"} {
		t.Run(damage, func(t *testing.T) {
			s, home, config, first := a4Setup(t, "codex-user", a4Manifest("first", "First."), false)
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), nativePackageYAML("second"))
			a4Install(t, s, home, "codex-user", second, false)
			lock, _ := loadLock(home)
			item, _ := findLockedPackage(lock, "second")
			var baseline approvedSnapshot
			if err := json.Unmarshal(readSyncFile(t, approvalPath(home, "second")), &baseline); err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "missing":
				if err := os.Remove(approvalPath(home, "second")); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				writeSyncFile(t, approvalPath(home, "second"), "corrupt")
			case "no-digest":
				item.Approval = ""
			case "wrong-files":
				baseline.Files["STYLE.yaml"] = []byte(nativePackageYAML("impostor"))
			case "legacy-native":
				baseline.Native = nil
			}
			if damage == "wrong-files" || damage == "legacy-native" {
				data, _ := json.Marshal(baseline)
				writeSyncFile(t, approvalPath(home, "second"), string(data))
				item.Approval = hashBytes(data)
			}
			for i := range lock.Packages {
				if lock.Packages[i].Name == "second" {
					lock.Packages[i] = item
				}
			}
			if err := writeLock(home, lock); err != nil {
				t.Fatal(err)
			}
			writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), a4Manifest("first", "Changed."))
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
			a4ExpectError(t, result, err, "update-baseline-unavailable")
			a4AssertState(t, before, home, config)
		})
	}
}

func TestM3A4UnselectedEffectsRequireAll(t *testing.T) {
	for _, effect := range []string{"dedup", "settings", "native-configuration", "native-trust"} {
		t.Run(effect, func(t *testing.T) {
			if strings.HasPrefix(effect, "native-") {
				requirePOSIXFixture(t, "unselected author native effects require POSIX")
			}
			s, home, config, first := a4Setup(t, "codex-user", a4Manifest("first", "First."), false)
			manifest := a4Manifest("second", "Second.")
			if effect == "settings" {
				manifest = strings.Replace(a4Manifest("second", "{{settings.voice}}"), "rules:", "settings: [{name: voice, default: ApprovedValue}]\nrules:", 1)
			} else if strings.HasPrefix(effect, "native") {
				manifest = nativePackageYAML("second")
			}
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), manifest)
			a4Install(t, s, home, "codex-user", second, false)
			next := a4Manifest("first", "First changed.")
			switch effect {
			case "dedup":
				next = a4Manifest("first", "Second.")
			case "settings":
				writeSyncFile(t, filepath.Join(home, "settings", "second.yaml"), "voice: UNSELECTED-PRIVATE-VALUE\n")
			case "native-configuration":
				path := filepath.Join(config, "hooks.json")
				writeSyncFile(t, path, strings.ReplaceAll(string(readSyncFile(t, path)), "printf earlier", "printf edited"))
			case "native-trust":
				path := filepath.Join(config, "config.toml")
				writeSyncFile(t, path, strings.ReplaceAll(string(readSyncFile(t, path)), "trusted_hash", "previous_hash"))
			}
			writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), next)
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
			a4ExpectError(t, result, err, "update-unselected-effect")
			if !strings.Contains(result.Diagnostics[len(result.Diagnostics)-1].Message, "second") {
				t.Fatal("missing affected package")
			}
			for _, d := range result.Diagnostics {
				if strings.Contains(d.Message, "UNSELECTED-PRIVATE-VALUE") || strings.Contains(d.Message, "printf") {
					t.Fatal("routine diagnostic leaked content")
				}
			}
			a4AssertState(t, before, home, config)
			result, err = s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true})
			if err != nil || !result.Changed || !slices.Equal(result.Selected, []string{"first", "second"}) {
				t.Fatalf("all=%+v %v", result, err)
			}
			if effect == "dedup" {
				alias := false
				for _, report := range result.Reports {
					if report.Package == "second" {
						for _, rule := range report.Plan.Rules {
							alias = alias || rule.DeduplicatedInto != ""
						}
					}
				}
				if !alias {
					t.Fatal("joint reports omit cross-package dedup")
				}
			}
		})
	}
}

func TestM3A4NativeChangesAndFirstLinkTrust(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX author native/login shell fixture; Windows rejects those capabilities")
	}
	for _, route := range a4Routes {
		for _, change := range []string{"add", "modify", "remove", "reorder", "first-link-trust"} {
			t.Run(route+"/"+change, func(t *testing.T) {
				beforeManifest := nativePackageYAML("first")
				if change == "add" {
					beforeManifest = a4Manifest("first", "Before.")
				}
				s, home, config, origin := a4Setup(t, route, beforeManifest, change == "first-link-trust")
				next := nativePackageYAML("first")
				switch change {
				case "modify":
					next = strings.ReplaceAll(next, "printf earlier", "printf changed-native")
				case "remove":
					next = a4Manifest("first", "After removal.")
				case "reorder":
					next = strings.ReplaceAll(next, "priority: 20.5", "priority: 90")
				}
				writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), next)
				before := a4State(t, home, config)
				preview, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first"})
				if err != nil || !preview.AwaitingConfirmation || !preview.Changes[0].NativeChanged && change != "first-link-trust" {
					t.Fatalf("preview=%+v %v", preview, err)
				}
				a4AssertState(t, before, home, config)
				result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
				if err != nil || !result.Changed {
					t.Fatalf("%+v %v", result, err)
				}
				if change == "reorder" && (len(result.Changes[0].NativeDiff) != 0 || len(result.Changes[0].NativeBefore) != 2 || result.Changes[0].NativeBefore[0].Rule == result.Changes[0].NativeAfter[0].Rule) {
					t.Fatal("native order change not fully disclosed")
				}
				if change == "first-link-trust" && !result.Changes[0].After.LinkTrusted {
					t.Fatal("first link trust not approved")
				}
				checks, err := s.checkNative(context.Background(), home, mustA4Lock(t, home), "")
				if err != nil {
					t.Fatal(err)
				}
				for _, check := range checks {
					if check.Status != "ok" || check.TrustStatus != "" && check.TrustStatus != "match" {
						t.Fatalf("native result=%+v", check)
					}
				}
			})
		}
	}
}

func mustA4Lock(t *testing.T, home string) lockDocument {
	t.Helper()
	lock, err := loadLock(home)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func TestM3A4CandidateGatesAndBlockedCLI(t *testing.T) {
	for _, gate := range []string{"version", "name", "schema", "runtime", "harness", "native", "template", "budget", "input-ceiling", "replace", "two-replace", "fallback-error"} {
		t.Run(gate, func(t *testing.T) {
			manifest := a4Manifest("first", "Long instructions to enforce the candidate budget.")
			s, home, config, origin := a4Setup(t, "codex-user", manifest, false)
			next := manifest
			switch gate {
			case "version":
				next = strings.ReplaceAll(next, "version: 1.0.0", "version: invalid")
			case "name":
				next = strings.ReplaceAll(next, "name: first", "name: other")
			case "schema":
				next = strings.ReplaceAll(next, "target: system", "target: invalid")
			case "runtime":
				next = strings.Replace(next, "rules:", "requires: {runtime: '>=99.0.0'}\nrules:", 1)
			case "harness":
				next = strings.Replace(next, "rules:", "requires: {harness: {codex: '>=99.0.0'}}\nrules:", 1)
			case "native":
				next = strings.ReplaceAll(nativePackageYAML("first"), "event: PreToolUse", "event: UnknownEvent")
			case "template":
				next = strings.ReplaceAll(next, "Long instructions to enforce the candidate budget.", "{{settings.missing}}")
			case "budget":
				next = strings.Replace(next, "rules:", "budget: {per_rule: 1000, per_event: 1, always_on: 600, on: error}\nrules:", 1)
			case "input-ceiling":
				d := s.definitions["codex"]
				d.Limits.Input = tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}
				s.definitions["codex"] = d
			case "replace", "two-replace":
				next = strings.ReplaceAll(next, "target: system", "target: system, mode: replace")
				if gate == "two-replace" {
					second := writeInstallPackage(t, filepath.Join(filepath.Dir(origin), "second"), strings.ReplaceAll(a4Manifest("second", "Other."), "target: system", "target: system, mode: replace"))
					request := InstallRequest{Home: home, Source: second, Yes: true, AllowReplace: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}
					if _, err := s.Install(context.Background(), request); err != nil {
						t.Fatal(err)
					}
				}
			case "fallback-error":
				next = "spec: style/v0.3\nname: first\nversion: 1.0.0\ndescription: blocked\nrules: []\nnative_hooks:\n  - {id: native, fallback: error, codex: {event: Stop, command: 'printf PRIVATE-COMMAND'}}\n"
				d := s.definitions["codex"]
				d.Facts = nonNativeFacts{d.Facts}
				s.definitions["codex"] = d
			}
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), next)
			t.Setenv("STYLE_HOME", home)
			before := a4State(t, home, config)
			var out, errOut bytes.Buffer
			exit := RunWithServiceInput(context.Background(), s, t.TempDir(), []string{"update", "first", "--json"}, approve(), &out, &errOut)
			var result UpdateResult
			if err := decodeReport(out.Bytes(), &result); err != nil || !strings.HasPrefix(errOut.String(), "style update: ") {
				t.Fatalf("single JSON: %s %s %v", &out, &errOut, err)
			}
			want := ExitInvalid
			if gate == "budget" || gate == "input-ceiling" || gate == "fallback-error" {
				want = ExitBlocked
			}
			if exit != want || result.Status != "error" || result.Changed {
				t.Fatalf("gate=%s exit=%d result=%+v", gate, exit, result)
			}
			a4AssertState(t, before, home, config)
			if gate == "replace" {
				result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true, AllowReplace: true})
				if err != nil || !result.Changed || !result.Changes[0].After.Replace {
					t.Fatalf("replace=%+v %v", result, err)
				}
			}
		})
	}
}

func TestM3A4GitCaptureAndDetachedPreview(t *testing.T) {
	s, home, _, f := a3Fixture(t, "codex-user")
	if _, err := s.Install(context.Background(), a3Request(home, "codex-user")); err != nil {
		t.Fatal(err)
	}
	manifest := strings.ReplaceAll(nativePackageYAML("native-test"), "printf earlier", "printf captured")
	captured := f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(manifest)}})
	previewCalls := 0
	var moved string
	result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "native-test", Confirm: func(preview UpdateResult) (bool, error) {
		previewCalls++
		if preview.Changes[0].After.Source.Git.Commit != captured {
			t.Fatal("wrong preview revision")
		}
		preview.Changes[0].After.Source.Git.Commit = strings.Repeat("f", 40)
		preview.Changes[0].After.Settings = append(preview.Changes[0].After.Settings, "intruder")
		preview.Selected[0] = "intruder"
		preview.Changes[0].SourceDiff[0].Diff = "mutated"
		if len(preview.Native) > 0 {
			preview.Native[0].Command = "printf mutated-preview"
		}
		preview.Reports[0].Plan.Rules[0].ID = "mutated"
		moved = f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(strings.ReplaceAll(manifest, "printf captured", "printf moved-after-confirm"))}})
		return true, nil
	}})
	if err != nil || !result.Changed || previewCalls != 1 || result.Selected[0] != "native-test" || result.Changes[0].After.Source.Git.Commit != captured || captured == moved || (len(result.Native) > 0 && result.Native[0].Command != "printf captured") {
		t.Fatalf("capture=%+v %v calls=%d", result, err, previewCalls)
	}
	lock := mustA4Lock(t, home)
	if lock.Packages[0].Source.Git.Commit != captured || !bytes.Contains(readSyncFile(t, filepath.Join(home, "packages", "native-test", "STYLE.yaml")), []byte("printf captured")) {
		t.Fatal("confirmation refetched moving source")
	}
}

func TestM3A4BatchValidationAndJointCompilation(t *testing.T) {
	for _, failure := range []string{"none", "missing-local", "invalid-local", "git-fetch"} {
		t.Run(failure, func(t *testing.T) {
			s, home, config, first := a4Setup(t, "claude-user", a4Manifest("first", "First."), false)
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), a4Manifest("second", "Second."))
			a4Install(t, s, home, "claude-user", second, true)
			f := testgit.New(t, "sha1", []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(a4Manifest("third", "Third."))}})
			s.sources = f.Resolver()
			if _, err := s.Install(context.Background(), a3Request(home, "claude-user")); err != nil {
				t.Fatal(err)
			}
			writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), a4Manifest("first", "Shared updated."))
			writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), a4Manifest("second", "Shared updated."))
			f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(a4Manifest("third", "Third updated."))}})
			switch failure {
			case "missing-local":
				if err := os.RemoveAll(second); err != nil {
					t.Fatal(err)
				}
			case "invalid-local":
				writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), "invalid")
			case "git-fetch":
				s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
					return source.Snapshot{}, &source.Error{Code: "source-fetch-failed", Message: "fixture unavailable"}
				})
			}
			for provider, definition := range s.definitions {
				definition.Detector = a2RejectDetector{t}
				s.definitions[provider] = definition
			}
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true})
			if failure != "none" {
				a4ExpectError(t, result, err, "")
				a4AssertState(t, before, home, config)
				return
			}
			if err != nil || !result.Changed || !slices.Equal(result.Selected, []string{"first", "second", "third"}) || len(result.Reports) != 3 {
				t.Fatalf("batch=%+v %v", result, err)
			}
			var index compile.Index
			if err := json.Unmarshal(readSyncFile(t, filepath.Join(home, "build", "index.json")), &index); err != nil {
				t.Fatal(err)
			}
			if index.Format != 1 || len(index.Packages) != 3 || index.Packages[0].Seq != 1 || index.Packages[2].Seq != 3 {
				t.Fatalf("index=%+v", index)
			}
			for _, events := range index.Harnesses {
				for _, entries := range events {
					for _, entry := range entries {
						if strings.HasPrefix(entry.Rule, "second/") {
							t.Fatal("joint dedup was skipped")
						}
					}
				}
			}
		})
	}
}

// Fix mtimes away from wall-clock time so restoration checks cannot pass
// because two writes happened within one filesystem timestamp tick.
func a4OldTimes(t *testing.T, home, config string) {
	t.Helper()
	when := time.Date(2020, 1, 2, 3, 4, 5, 123456789, time.UTC)
	for path := range a4State(t, home, config) {
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdateRejectsRequestsTheInstallationCannotSatisfy(t *testing.T) {
	s, home, _, _ := a4Setup(t, "codex-user", a4Manifest("first", "Before."), false)
	for _, request := range []UpdateRequest{
		{Home: home, Name: "missing", Yes: true},
		{Home: home, Name: "first", Ref: "refs/heads/main", Yes: true},
	} {
		result, err := s.Update(context.Background(), request)
		if err == nil || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "request-rejected" {
			t.Fatalf("%+v: diagnostics=%+v err=%v", request, result.Diagnostics, err)
		}
	}
}
