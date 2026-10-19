package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testgit"
	"gopkg.in/yaml.v3"
	"runtime"
)

func a4WriteRawLock(t *testing.T, home string, lock lockDocument) {
	t.Helper()
	data, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(home, "style.lock.yaml"), string(data))
}

func TestM3A4LegacyLockAndApprovalCompatibility(t *testing.T) {
	for _, route := range a4Routes {
		for _, legacy := range []string{"no-approval", "no-native-field", "implicit-format", "format-two-local"} {
			t.Run(route+"/"+legacy, func(t *testing.T) {
				s, home, config, origin := a4Setup(t, route, a4Manifest("first", "Old."), false)
				lock := mustA4Lock(t, home)
				switch legacy {
				case "no-approval":
					lock.Packages[0].Approval = ""
					if err := os.Remove(approvalPath(home, "first")); err != nil {
						t.Fatal(err)
					}
				case "no-native-field":
					var approved map[string]json.RawMessage
					if err := json.Unmarshal(readSyncFile(t, approvalPath(home, "first")), &approved); err != nil {
						t.Fatal(err)
					}
					delete(approved, "native")
					data, _ := json.Marshal(approved)
					writeSyncFile(t, approvalPath(home, "first"), string(data))
					lock.Packages[0].Approval = hashBytes(data)
				case "format-two-local":
					lock.Format = 2
				}
				if err := writeLock(home, lock); err != nil {
					t.Fatal(err)
				}
				if legacy == "implicit-format" {
					path := filepath.Join(home, "style.lock.yaml")
					writeSyncFile(t, path, strings.Replace(string(readSyncFile(t, path)), "format: 1\n", "", 1))
				}
				writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), a4Manifest("first", "Updated."))
				for provider, d := range s.definitions {
					d.Detector = a2RejectDetector{t}
					s.definitions[provider] = d
				}
				before := a4State(t, home, config)
				preview, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first"})
				if err != nil || !preview.AwaitingConfirmation || preview.Changes[0].BaselineAvailable != (legacy != "no-approval") {
					t.Fatalf("legacy preview=%+v %v", preview, err)
				}
				a4AssertState(t, before, home, config)
				result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
				if err != nil || !result.Changed {
					t.Fatalf("legacy apply=%+v %v", result, err)
				}
				updated := mustA4Lock(t, home)
				if updated.Format != lock.Format || updated.Packages[0].Seq != 1 || updated.Packages[0].Approval == "" {
					t.Fatalf("legacy lock=%+v", updated)
				}
				if _, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Yes: true}); err != nil {
					t.Fatal(err)
				}
				before = a4State(t, home, config)
				result, err = s.Update(context.Background(), UpdateRequest{Home: home, All: true})
				if err != nil || result.Status != "unchanged" {
					t.Fatalf("repeat=%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestM3A4UnselectedOrdinaryLegacyApproval(t *testing.T) {
	s, home, _, first := a4Setup(t, "codex-user", a4Manifest("first", "First."), false)
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), a4Manifest("second", "Second."))
	a4Install(t, s, home, "codex-user", second, false)
	var baseline map[string]json.RawMessage
	if err := json.Unmarshal(readSyncFile(t, approvalPath(home, "second")), &baseline); err != nil {
		t.Fatal(err)
	}
	delete(baseline, "native")
	oldBytes, _ := json.Marshal(baseline)
	writeSyncFile(t, approvalPath(home, "second"), string(oldBytes))
	lock := mustA4Lock(t, home)
	lock.Packages[1].Approval = hashBytes(oldBytes)
	if err := writeLock(home, lock); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), a4Manifest("first", "First updated."))
	result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
	if err != nil || !result.Changed || !bytes.Equal(oldBytes, readSyncFile(t, approvalPath(home, "second"))) {
		t.Fatalf("ordinary legacy=%+v %v", result, err)
	}
	if mustA4Lock(t, home).Packages[1].Approval != hashBytes(oldBytes) {
		t.Fatal("rewrote unselected legacy approval")
	}
}

func TestM3A4InvalidOldLocksFailBeforeSourceAcquisition(t *testing.T) {
	for _, damage := range []string{"format", "name", "duplicate", "sequence", "source-path", "kind", "link", "hash", "escape"} {
		t.Run(damage, func(t *testing.T) {
			s, home, config, _ := a4Setup(t, "codex-user", a4Manifest("first", "Old."), false)
			lock := mustA4Lock(t, home)
			switch damage {
			case "format":
				lock.Format = 9
			case "name":
				lock.Packages[0].Name = "../escape"
			case "duplicate":
				lock.Packages = append(lock.Packages, lock.Packages[0])
			case "sequence":
				lock.Packages[0].Seq = 0
			case "source-path":
				lock.Packages[0].Source.Path = "relative"
			case "kind":
				lock.Packages[0].Source.Kind = "other"
			case "link":
				lock.Packages[0].Source.Link = true
			case "hash":
				lock.Packages[0].Files["STYLE.yaml"] = "bad"
			case "escape":
				lock.Packages[0].Files["../escape"] = lock.Packages[0].Files["STYLE.yaml"]
			}
			a4WriteRawLock(t, home, lock)
			s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
				t.Fatal("invalid lock acquired a source")
				return source.Snapshot{}, nil
			})
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true})
			a4ExpectError(t, result, err, "lock-invalid")
			a4AssertState(t, before, home, config)
		})
	}
}

func TestM3A4GitManagedDriftBeforeFetchAndUnselectedGit(t *testing.T) {
	for _, drift := range []string{"edit", "missing", "symlink", "unselected"} {
		t.Run(drift, func(t *testing.T) {
			s, home, config, f := a3Fixture(t, "codex-user")
			if _, err := s.Install(context.Background(), a3Request(home, "codex-user")); err != nil {
				t.Fatal(err)
			}
			first := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "first-source"), a4Manifest("first", "First."))
			a4Install(t, s, home, "codex-user", first, false)
			managed := filepath.Join(home, "packages", "native-test", "STYLE.yaml")
			switch drift {
			case "edit", "unselected":
				writeSyncFile(t, managed, strings.ReplaceAll(nativePackageYAML("native-test"), "printf earlier", "printf UNAPPROVED-GIT-EDIT"))
			case "missing":
				if err := os.Remove(managed); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(managed); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(first, "STYLE.yaml"), managed); err != nil {
					t.Fatal(err)
				}
			}
			calls := len(f.Commands)
			s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
				t.Fatal("drifted/unselected Git was fetched")
				return source.Snapshot{}, nil
			})
			before := a4State(t, home, config)
			request := UpdateRequest{Home: home, Name: "native-test", Yes: true}
			if drift == "unselected" {
				request.Name = "first"
				writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), a4Manifest("first", "First updated."))
			}
			result, err := s.Update(context.Background(), request)
			if drift == "unselected" {
				if err != nil || !result.Changed || !bytes.Equal([]byte(before[managed].Data), readSyncFile(t, managed)) {
					t.Fatalf("unselected Git=%+v %v", result, err)
				}
			} else {
				a4ExpectError(t, result, err, "git-snapshot-changed")
				a4AssertState(t, before, home, config)
			}
			if len(f.Commands) != calls {
				t.Fatal("Git was consulted")
			}
		})
	}
}

func TestM3A4ConfirmationDeclineErrorAndRepeatCapture(t *testing.T) {
	for _, answer := range []string{"decline", "error", "approve", "separate-call"} {
		t.Run(answer, func(t *testing.T) {
			s, home, config, origin := a4Setup(t, "codex-user", a4Manifest("first", "Old."), false)
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), a4Manifest("first", "First capture."))
			before := a4State(t, home, config)
			calls := 0
			request := UpdateRequest{Home: home, Name: "first", Confirm: func(preview UpdateResult) (bool, error) {
				calls++
				if answer == "error" {
					return false, errors.New("PRIVATE-CALLBACK-CONTENT")
				}
				return answer == "approve", nil
			}}
			result, err := s.Update(context.Background(), request)
			if calls != 1 {
				t.Fatal("repeated confirmation")
			}
			switch answer {
			case "error":
				a4ExpectError(t, result, err, "confirmation-failed")
				if strings.Contains(result.Diagnostics[0].Message, "PRIVATE-CALLBACK-CONTENT") {
					t.Fatal("callback contents leaked")
				}
				a4AssertState(t, before, home, config)
			case "decline", "separate-call":
				if err != nil || !result.AwaitingConfirmation || result.Changed {
					t.Fatalf("decline=%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				if answer == "separate-call" {
					writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), a4Manifest("first", "Second capture."))
					result, err = s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
					if err != nil || !result.Changed || !strings.Contains(result.Changes[0].SourceDiff[0].Diff, "Second capture.") {
						t.Fatalf("fresh call=%+v %v", result, err)
					}
				}
			case "approve":
				if err != nil || !result.Changed {
					t.Fatalf("approve=%+v %v", result, err)
				}
			}
		})
	}
}

func a4Golden(t *testing.T, name, text, root string) {
	if runtime.GOOS == "windows" {
		t.Run("POSIX-golden/"+name, func(t *testing.T) {
			t.Skip("D11: POSIX native preview and path golden; parent asserts command lifecycle and JSON")
		})
		return
	}
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{canonical, root} {
		text = strings.ReplaceAll(text, path, "$FIXTURE")
	}
	path := filepath.Join("..", "..", "testdata", "update", name)
	if os.Getenv("STYLE_M3_A4_RECORD") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := string(readSyncFile(t, path)); got != text {
		t.Fatalf("%s differs\n%s", name, text)
	}
}

func TestM3A4CommandGoldenAndSingleJSON(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonOutput], func(t *testing.T) {
			s, home, config, origin := a4Setup(t, "codex-user", a4Manifest("first", "Before."), false)
			t.Setenv("STYLE_HOME", home)
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), a4Manifest("first", "After."))
			suffix := "txt"
			flags := []string{}
			if jsonOutput {
				suffix = "json"
				flags = []string{"--json"}
			}
			for _, phase := range []string{"preview", "apply", "unchanged", "error"} {
				args := append([]string{"update", "first"}, flags...)
				var input io.Reader
				if phase == "apply" {
					input = approve()
				}
				if phase == "error" {
					args[1] = "missing"
				}
				before := a4State(t, home, config)
				var out, errOut bytes.Buffer
				exit := RunWithServiceInput(context.Background(), s, t.TempDir(), args, input, &out, &errOut)
				want := ExitOK
				if phase == "preview" || phase == "error" {
					want = ExitInvalid
				}
				if exit != want {
					t.Fatalf("phase %s exit=%d stdout=%s stderr=%s", phase, exit, &out, &errOut)
				}
				if phase != "apply" {
					a4AssertState(t, before, home, config)
				}
				if jsonOutput {
					decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
					var result UpdateResult
					if err := decoder.Decode(&result); err != nil || decoder.More() || result.Command != "update" || result.Format != 1 {
						t.Fatalf("JSON=%s err=%v stderr=%s", &out, err, &errOut)
					}
				}
				a4Golden(t, phase+".stdout."+suffix, out.String(), filepath.Dir(home))
				a4Golden(t, phase+".stderr."+suffix, errOut.String(), filepath.Dir(home))
			}
		})
	}
}

func TestM3A4ReportsSortByExistingSequence(t *testing.T) {
	s, home, _, first := a4Setup(t, "codex-user", a4Manifest("first", "First."), false)
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), a4Manifest("second", "Second."))
	a4Install(t, s, home, "codex-user", second, false)
	lock := mustA4Lock(t, home)
	lock.Packages[0], lock.Packages[1] = lock.Packages[1], lock.Packages[0]
	a4WriteRawLock(t, home, lock)
	writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), a4Manifest("first", "First updated."))
	writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), a4Manifest("second", "Second updated."))
	firstResult, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true})
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true})
	if err != nil || !reflect.DeepEqual(firstResult, secondResult) || !slices.Equal(firstResult.Selected, []string{"first", "second"}) || firstResult.Changes[0].Sequence != 1 || firstResult.Reports[1].Package != "second" {
		t.Fatalf("ordering=%+v %v", firstResult, err)
	}
	if _, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if mustA4Lock(t, home).Packages[0].Name != "second" {
		t.Fatal("reordered lock storage unnecessarily")
	}
}

func TestM3A4PinnedOIDCannotFallBackToBranchOrCache(t *testing.T) {
	s, home, config, _, _ := nativeInstallFixture(t, "codex-user")
	f := testgit.New(t, "sha1", []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(a4Manifest("first", "Pinned."))}})
	s.sources = f.Resolver()
	request := a3Request(home, "codex-user")
	request.Ref = f.Commit
	if _, err := s.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	tree := f.Run(nil, "rev-parse", "refs/heads/main^{tree}")
	unrelated := f.Run([]byte("unrelated commit\n"), "commit-tree", tree)
	f.Run(nil, "update-ref", "refs/heads/main", unrelated)
	f.Run(nil, "reflog", "expire", "--expire=all", "--all")
	f.Run(nil, "gc", "--prune=now")
	before := a4State(t, home, config)
	result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
	a4ExpectError(t, result, err, "source-fetch-failed")
	if result.Changes[0].After != nil {
		t.Fatal("unreachable pinned commit used a cached/branch candidate")
	}
	a4AssertState(t, before, home, config)
}

func TestM3A4LockedHarnessesAndLayout(t *testing.T) {
	for _, binding := range []string{"both-harnesses", "legacy-static-layout", "missing-route", "missing-provider"} {
		t.Run(binding, func(t *testing.T) {
			s, home, config, origin, _ := nativeInstallFixture(t, "codex-user")
			request := InstallRequest{Home: home, Source: origin, Yes: true, Harnesses: []string{"codex", "claude-code"},
				Routes: map[string]string{"codex": "codex-user", "claude-code": "claude-user"}, FailClosed: []string{"claude-code"}}
			if _, err := s.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			lock := mustA4Lock(t, home)
			if len(lock.Harnesses) != 2 {
				t.Fatal("missing binding fixture")
			}
			if binding == "legacy-static-layout" {
				old := lock.Harnesses["claude-code"]
				old.StaticLayout = "claude_md"
				old.StaticFiles = nil
				lock.Harnesses["claude-code"] = old
			} else if binding == "missing-route" {
				old := lock.Harnesses["codex"]
				old.Route = "unavailable-route"
				lock.Harnesses["codex"] = old
			} else if binding == "missing-provider" {
				lock.Harnesses["unavailable"] = lock.Harnesses["codex"]
			}
			if err := writeLock(home, lock); err != nil {
				t.Fatal(err)
			}
			writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), strings.ReplaceAll(nativePackageYAML("native-test"), "printf earlier", "printf captured-multi-harness"))
			for p, d := range s.definitions {
				d.Detector = a2RejectDetector{t}
				s.definitions[p] = d
			}
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true})
			if binding != "both-harnesses" {
				a4ExpectError(t, result, err, "update-binding-changed")
				a4AssertState(t, before, home, config)
				return
			}
			if err != nil || !result.Changed || len(result.Routes) != 2 || len(result.Reports) != 2 || len(result.Native) != fixtureNativeCount(4) {
				t.Fatalf("multi binding=%+v %v", result, err)
			}
			updated := mustA4Lock(t, home)
			if updateBindingChanged(lock.Harnesses, updated.Harnesses) || !updated.Harnesses["claude-code"].FailClosed {
				t.Fatal("changed a locked harness binding")
			}
			checks, err := s.checkNative(context.Background(), home, updated, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range checks {
				if check.Status != "ok" || check.TrustStatus != "" && check.TrustStatus != "match" {
					t.Fatalf("native=%+v", check)
				}
			}
		})
	}
}
