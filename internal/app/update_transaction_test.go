package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

func a4Batch(t *testing.T, route string) (*Service, string, string, []string) {
	t.Helper()
	manifest := strings.Replace(nativePackageYAML("first"), "rules:", "settings: [{name: voice, default: Original}]\nrules:\n  - {id: static, target: system, text: 'Static {{settings.voice}}.'}", 1)
	s, home, config, first := a4Setup(t, route, manifest, false)
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second-source"), a4Manifest("second", "Second original."))
	a4Install(t, s, home, route, second, false)
	manifest = strings.ReplaceAll(manifest, "printf earlier", "printf changed")
	manifest = strings.ReplaceAll(manifest, "Static {{settings.voice}}.", "Changed static {{settings.voice}}.")
	manifest = strings.ReplaceAll(manifest, "default: Original}]", "default: Changed}, {name: added, default: AddedDefault}]")
	writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), manifest)
	writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), a4Manifest("second", "Second updated."))
	for path := range a4State(t, home, config) {
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	a4OldTimes(t, home, config)
	return s, home, config, []string{first, second}
}

type a4FaultSwap struct {
	*packageSwap
	failApply    bool
	failRollback bool
}

func (s *a4FaultSwap) apply() error {
	if s.failApply {
		if err := os.RemoveAll(s.staged); err != nil {
			return err
		}
	}
	return s.packageSwap.apply()
}
func (s *a4FaultSwap) rollback() error {
	if s.failRollback && s.applied {
		return fmt.Errorf("injected restore failure; backup retained at %s", s.backup)
	}
	return s.packageSwap.rollback()
}

type a4CancelAt struct {
	context.Context
	checks, at int
	reached    bool
}

func (c *a4CancelAt) Err() error {
	c.checks++
	if c.checks >= c.at {
		c.reached = true
		return context.Canceled
	}
	return c.Context.Err()
}

func TestM3A4EachPackageFailureRestoresBatch(t *testing.T) {
	for _, route := range a4Routes {
		for _, stage := range []string{"stage-first", "stage-second", "apply-first", "apply-second"} {
			t.Run(route+"/"+stage, func(t *testing.T) {
				s, home, config, _ := a4Batch(t, route)
				before := a4State(t, home, config)
				prepared, err := s.prepareUpdate(context.Background(), UpdateRequest{Home: home, All: true, Yes: true})
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				deps := updateDependencies{stage: func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
					calls++
					if stage == "stage-"+name {
						return nil, errors.New("injected staging failure")
					}
					swap, err := stagePackageFiles(home, name, single, files)
					if err != nil {
						return nil, err
					}
					return &a4FaultSwap{packageSwap: swap, failApply: stage == "apply-"+name}, nil
				}}
				result, err := applyUpdate(context.Background(), prepared, deps)
				if err == nil || result.Changed || calls == 0 {
					t.Fatalf("result=%+v err=%v stage calls=%d", result, err, calls)
				}
				a4AssertState(t, before, home, config)
			})
		}
	}
}

func TestM3A4EachFileFailureRestoresBatch(t *testing.T) {
	for _, route := range a4Routes {
		// Enumerate the actual production plan, rather than guessing which
		// native/static/trust files a renderer will need on each route.
		s, home, _, _ := a4Batch(t, route)
		plan, err := s.prepareUpdate(context.Background(), UpdateRequest{Home: home, All: true})
		if err != nil {
			t.Fatal(err)
		}
		var labels []string
		for _, file := range plan.files {
			rel, err := filepath.Rel(filepath.Dir(home), file.Path)
			if err != nil {
				t.Fatal(err)
			}
			labels = append(labels, rel)
		}
		labels = append(labels, "after-final-file")
		for index, label := range labels {
			t.Run(route+"/"+label, func(t *testing.T) {
				s, home, config, _ := a4Batch(t, route)
				before := a4State(t, home, config)
				prepared, err := s.prepareUpdate(context.Background(), UpdateRequest{Home: home, All: true})
				if err != nil {
					t.Fatal(err)
				}
				if len(prepared.files)+1 != len(labels) {
					t.Fatal("non-deterministic file plan")
				}
				var fault *a4CancelAt
				deps := updateDependencies{files: func(ctx context.Context, files []install.PlannedFile) error {
					all := append([]install.PlannedFile(nil), files...)
					// A final check also exercises rollback after the real lock or
					// last renderer file has already been applied.
					all = append(all, install.PlannedFile{Path: filepath.Join(home, ".a4-sentinel"), Root: home, After: []byte("never committed"), Mode: 0o600})
					fault = &a4CancelAt{Context: ctx, at: 2*len(all) + index + 1}
					return install.ApplyFiles(fault, all)
				}}
				result, err := applyUpdate(context.Background(), prepared, deps)
				if !errors.Is(err, context.Canceled) || result.Changed || fault == nil || !fault.reached {
					t.Fatalf("%+v %v fault=%+v", result, err, fault)
				}
				a4AssertState(t, before, home, config)
			})
		}
	}
}

func TestM3A4RollbackFailuresAreReportedAndRetained(t *testing.T) {
	s, home, config, _ := a4Batch(t, "codex-user")
	before := a4State(t, home, config)
	prepared, err := s.prepareUpdate(context.Background(), UpdateRequest{Home: home, All: true})
	if err != nil {
		t.Fatal(err)
	}
	var failed *a4FaultSwap
	deps := updateDependencies{
		stage: func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
			swap, err := stagePackageFiles(home, name, single, files)
			if err != nil {
				return nil, err
			}
			fault := &a4FaultSwap{packageSwap: swap, failRollback: name == "first"}
			if fault.failRollback {
				failed = fault
			}
			return fault, nil
		},
		files: func(context.Context, []install.PlannedFile) error { return context.Canceled },
	}
	result, err := applyUpdate(context.Background(), prepared, deps)
	if err == nil || result.Changed || updateDiagnostic(err).Code != "rollback-failed" || !strings.Contains(err.Error(), "backup retained at") || failed == nil {
		t.Fatalf("%+v %v", result, err)
	}
	if !bytes.Contains(readSyncFile(t, filepath.Join(failed.backup, "STYLE.yaml")), []byte("printf earlier")) {
		t.Fatal("recovery backup was deleted")
	}
	after := a4State(t, home, config)
	for path, state := range before {
		if strings.Contains(filepath.ToSlash(path), "/packages/first/") {
			continue
		}
		if after[path] != state {
			t.Fatalf("another rollback was skipped: %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(home, managementName)); !os.IsNotExist(err) {
		t.Fatal("failed rollback leaked the management guard")
	}
}

func TestM3A4ConfirmationRejectsEveryStaleInput(t *testing.T) {
	for _, input := range []string{"source", "source-identity", "source-mode", "lock", "lock-identity", "lock-mtime", "approval", "settings", "managed", "unselected-managed", "index", "explain", "hook", "native-trust", "parent", "new-file"} {
		t.Run(input, func(t *testing.T) {
			s, home, config, origins := a4Batch(t, "codex-user")
			paths := map[string]string{"source": filepath.Join(origins[0], "STYLE.yaml"), "source-identity": filepath.Join(origins[0], "STYLE.yaml"), "source-mode": filepath.Join(origins[0], "STYLE.yaml"),
				"lock": filepath.Join(home, "style.lock.yaml"), "lock-identity": filepath.Join(home, "style.lock.yaml"), "lock-mtime": filepath.Join(home, "style.lock.yaml"),
				"approval": approvalPath(home, "first"), "settings": filepath.Join(home, "settings", "first.yaml"), "managed": filepath.Join(home, "packages", "first", "STYLE.yaml"),
				"unselected-managed": filepath.Join(home, "packages", "second", "STYLE.yaml"), "index": filepath.Join(home, "build", "index.json"), "explain": filepath.Join(home, "build", "explain.json"),
				"hook": filepath.Join(config, "hooks.json"), "native-trust": filepath.Join(config, "config.toml")}
			var expected map[string]a4FileState
			request := UpdateRequest{Home: home, Name: "first", Confirm: func(UpdateResult) (bool, error) {
				path := paths[input]
				switch input {
				case "source-mode":
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					mode := info.Mode().Perm() ^ 0o044
					if runtime.GOOS == "windows" {
						mode = info.Mode().Perm() ^ 0o222
					}
					if err := os.Chmod(path, mode); err != nil {
						t.Fatal(err)
					}
				case "lock-mtime":
					if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
						t.Fatal(err)
					}
				case "source-identity", "lock-identity":
					data := readSyncFile(t, path)
					if err := os.Rename(path, path+".saved"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, data, 0o640); err != nil {
						t.Fatal(err)
					}
				case "parent":
					path := filepath.Join(home, "build")
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					if runtime.GOOS == "windows" {
						if err := os.Rename(path, path+"-previous"); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(path, 0700); err != nil {
							t.Fatal(err)
						}
					} else if err := os.Chmod(path, info.Mode().Perm()^0o055); err != nil {
						t.Fatal(err)
					}
				case "new-file":
					writeSyncFile(t, filepath.Join(home, "approved", "surprise.json"), "intervening edit")
				default:
					writeSyncFile(t, path, string(readSyncFile(t, path))+"\nintervening edit\n")
				}
				expected = a4State(t, home, config)
				return true, nil
			}}
			result, err := s.Update(context.Background(), request)
			a4ExpectError(t, result, err, "installation-changed")
			if expected == nil {
				t.Fatal("confirmation did not run")
			}
			a4AssertState(t, expected, home, config)
		})
	}
}

func TestM3A4UpdateSharesGuardWithOtherWriters(t *testing.T) {
	for _, command := range []string{"install", "update", "sync", "trust", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			s, home, config, origins := a4Batch(t, "codex-user")
			// A missing derived artifact ensures every writer has a commit to
			// make, including an otherwise unchanged sync/trust.
			if err := os.Remove(filepath.Join(home, "build", "explain.json")); err != nil {
				t.Fatal(err)
			}
			before := a4State(t, home, config)
			guard, err := acquireManagement(context.Background(), home)
			if err != nil {
				t.Fatal(err)
			}
			var runErr error
			switch command {
			case "install":
				third := writeInstallPackage(t, filepath.Join(filepath.Dir(origins[0]), "third-source"), a4Manifest("third", "Third."))
				_, runErr = s.Install(context.Background(), InstallRequest{Home: home, Source: third, Harnesses: []string{"codex"}, Yes: true})
			case "update":
				_, runErr = s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true})
			case "sync":
				_, runErr = s.Sync(context.Background(), SyncRequest{Home: home})
			case "trust":
				_, runErr = s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Yes: true})
			case "uninstall":
				_, runErr = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true})
			}
			if !errors.Is(runErr, errManagementBusy) {
				t.Fatalf("%s ignored live guard: %v", command, runErr)
			}
			if err := guard.close(); err != nil {
				t.Fatal(err)
			}
			a4AssertState(t, before, home, config)
		})
	}
	// Exercise the other direction while the actual update commit owns it.
	t.Run("during-update-commit", func(t *testing.T) {
		s, home, config, _ := a4Batch(t, "codex-user")
		prepared, err := s.prepareUpdate(context.Background(), UpdateRequest{Home: home, All: true})
		if err != nil {
			t.Fatal(err)
		}
		called := false
		deps := updateDependencies{files: func(ctx context.Context, files []install.PlannedFile) error {
			called = true
			_, err := acquireManagement(ctx, home)
			if !errors.Is(err, errManagementBusy) {
				t.Fatalf("update did not own shared commit guard: %v", err)
			}
			return install.ApplyFiles(ctx, files)
		}}
		result, err := applyUpdate(context.Background(), prepared, deps)
		if err != nil || !called || !result.Changed {
			t.Fatalf("commit=%+v %v called=%t", result, err, called)
		}
		if len(a4State(t, home, config)) == 0 {
			t.Fatal("empty result")
		}
	})
}

func TestM3A4CancellationGuardRecoveryAndMaintenanceIsolation(t *testing.T) {
	for _, moment := range []string{"before", "confirmation", "stage", "files", "stale-guard", "preview-unlocked"} {
		t.Run(moment, func(t *testing.T) {
			s, home, config, _ := a4Batch(t, "codex-user")
			for _, rel := range []string{"state/expired.json", "stats/aggregate.json", "probe/expired.json", "probe/receipts/pending.json"} {
				writeSyncFile(t, filepath.Join(home, filepath.FromSlash(rel)), "runtime-private-content")
			}
			before := a4State(t, home, config)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := UpdateRequest{Home: home, All: true, Yes: true}
			if moment == "before" {
				cancel()
			}
			if moment == "confirmation" || moment == "preview-unlocked" {
				request.Yes = false
				request.Confirm = func(UpdateResult) (bool, error) {
					if _, err := os.Stat(filepath.Join(home, managementName)); !os.IsNotExist(err) {
						t.Fatal("guard held during confirmation")
					}
					if moment == "confirmation" {
						cancel()
					}
					return true, nil
				}
			}
			if moment == "stale-guard" {
				// A reaped real OS process identity is the existing stale-owner
				// proof; age alone is never used for reclamation.
				writeSyncFile(t, filepath.Join(home, managementName), fmt.Sprintf("{\"pid\":%d,\"birth\":\"terminated-fixture\",\"nonce\":\"%s\"}", 2147483000, strings.Repeat("a", 32)))
				if _, err := agenthooks.ReadProcessIdentity(2147483000); !errors.Is(err, agenthooks.ErrProcessGone) {
					t.Fatalf("fixture PID is not proven gone: %v", err)
				}
			}
			var result UpdateResult
			var err error
			if moment == "stage" || moment == "files" {
				prepared, prepareErr := s.prepareUpdate(ctx, request)
				if prepareErr != nil {
					t.Fatal(prepareErr)
				}
				deps := updateDependencies{}
				if moment == "stage" {
					deps.stage = func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
						cancel()
						return stagePackageFiles(home, name, single, files)
					}
				} else {
					deps.files = func(ctx context.Context, files []install.PlannedFile) error {
						cancel()
						return install.ApplyFiles(ctx, files)
					}
				}
				result, err = applyUpdate(ctx, prepared, deps)
			} else {
				result, err = s.Update(ctx, request)
			}
			if moment == "stale-guard" || moment == "preview-unlocked" {
				if err != nil || !result.Changed {
					t.Fatalf("success=%+v %v", result, err)
				}
				after := a4State(t, home, config)
				for path, state := range before {
					if strings.Contains(path, "/state/") || strings.Contains(path, "/stats/") || strings.Contains(path, "/probe/") {
						if after[path] != state {
							t.Fatalf("update maintained runtime data: %s", path)
						}
					}
				}
			} else {
				if !errors.Is(err, context.Canceled) || result.Changed {
					t.Fatalf("cancel=%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
			}
		})
	}
}

func TestM3A4FailureReportClassification(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		code string
	}{
		{"busy", errManagementBusy, "management-busy"}, {"stale", errManagementChanged, "installation-changed"},
		{"canceled", context.Canceled, "canceled"}, {"deadline", context.DeadlineExceeded, "canceled"},
		{"rollback", &updateRollbackError{errors.New("backup retained at /private/recovery")}, "rollback-failed"},
		{"fallback", &BlockedError{"plan for first is blocked on codex"}, "plan-blocked"},
		{"budget", &BlockedError{"T003 tool.pre blocks install"}, "T003"},
		{"safe-default", errors.New("PRIVATE-COMMAND\x1b[31m PRIVATE-SETTING"), "update-failed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := updateDiagnostic(c.err)
			if d.Code != c.code || strings.Contains(d.Message, "PRIVATE-") || strings.ContainsRune(d.Message, '\x1b') {
				t.Fatalf("diagnostic=%+v", d)
			}
		})
	}
}

func TestM3A4PrivateInputsCannotAliasUncapturedValues(t *testing.T) {
	for _, input := range []string{"lock", "approved-dir", "approval", "unselected-approval", "settings-dir", "settings", "build-dir", "index", "explain"} {
		t.Run(input, func(t *testing.T) {
			s, home, config, _ := a4Batch(t, "codex-user")
			paths := map[string]string{"lock": filepath.Join(home, "style.lock.yaml"), "approved-dir": filepath.Join(home, "approved"),
				"approval": approvalPath(home, "first"), "unselected-approval": approvalPath(home, "second"),
				"settings-dir": filepath.Join(home, "settings"), "settings": filepath.Join(home, "settings", "first.yaml"),
				"build-dir": filepath.Join(home, "build"), "index": filepath.Join(home, "build", "index.json"), "explain": filepath.Join(home, "build", "explain.json")}
			path := paths[input]
			outside := filepath.Join(filepath.Dir(home), "external-"+input)
			if err := os.Rename(path, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			before := a4State(t, home, config)
			result, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "first", Yes: true})
			a4ExpectError(t, result, err, "installation-changed")
			a4AssertState(t, before, home, config)
		})
	}
}
