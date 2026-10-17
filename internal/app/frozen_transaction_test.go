package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks/install"
)

func a5Batch(t *testing.T, route string) (*Service, string, string, []string) {
	t.Helper()
	s, home, config, origin := a4Setup(t, route, nativePackageYAML("first"), false)
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(origin), "second-source"), a4Manifest("second", "Second locked."))
	a4Install(t, s, home, route, second, false)
	for _, name := range []string{"first", "second"} {
		writeSyncFile(t, filepath.Join(home, "packages", name, "STYLE.yaml"), "corrupt "+name)
		if err := os.Chmod(filepath.Join(home, "packages", name, "STYLE.yaml"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
	a4OldTimes(t, home, config)
	a5Offline(t, s)
	return s, home, config, []string{origin, second}
}
func TestM3A5EachPackageFailureRestoresBatch(t *testing.T) {
	for _, route := range a4Routes {
		for _, stage := range []string{"stage-first", "stage-second", "apply-first", "apply-second"} {
			t.Run(route+"/"+stage, func(t *testing.T) {
				s, home, config, _ := a5Batch(t, route)
				before := a4State(t, home, config)
				check := a5Lock(t, home)
				p, err := s.prepareFrozen(context.Background(), FrozenRequest{Home: home})
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				deps := updateDependencies{stage: func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
					calls++
					if stage == "stage-"+name {
						return nil, errors.New("injected frozen stage failure")
					}
					swap, err := stagePackageFiles(home, name, single, files)
					if err != nil {
						return nil, err
					}
					return &a4FaultSwap{packageSwap: swap, failApply: stage == "apply-"+name}, nil
				}}
				result, err := applyUpdate(context.Background(), p.transaction, deps)
				if err == nil || result.Changed || calls == 0 {
					t.Fatalf("%+v %v calls=%d", result, err, calls)
				}
				a4AssertState(t, before, home, config)
				check()
			})
		}
	}
}
func TestM3A5EachFileFailureRestoresBatch(t *testing.T) {
	for _, route := range a4Routes {
		s, home, _, _ := a5Batch(t, route)
		p, err := s.prepareFrozen(context.Background(), FrozenRequest{Home: home})
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index <= len(p.transaction.files); index++ {
			t.Run(fmt.Sprintf("%s/file-%d", route, index), func(t *testing.T) {
				s, home, config, _ := a5Batch(t, route)
				before := a4State(t, home, config)
				check := a5Lock(t, home)
				p, err := s.prepareFrozen(context.Background(), FrozenRequest{Home: home})
				if err != nil {
					t.Fatal(err)
				}
				var fault *a4CancelAt
				deps := updateDependencies{files: func(ctx context.Context, files []install.PlannedFile) error {
					all := append([]install.PlannedFile(nil), files...)
					all = append(all, install.PlannedFile{Path: filepath.Join(home, ".a5-sentinel"), Root: home, After: []byte("uncommitted"), Mode: 0o600})
					fault = &a4CancelAt{Context: ctx, at: 2*len(all) + index + 1}
					return install.ApplyFiles(fault, all)
				}}
				result, err := applyUpdate(context.Background(), p.transaction, deps)
				if !errors.Is(err, context.Canceled) || result.Changed || fault == nil || !fault.reached {
					t.Fatalf("%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				check()
			})
		}
	}
}
func TestM3A5RollbackFailureRetainsBackup(t *testing.T) {
	s, home, config, _ := a5Batch(t, "codex-user")
	p, err := s.prepareFrozen(context.Background(), FrozenRequest{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	before := a4State(t, home, config)
	var failed *a4FaultSwap
	deps := updateDependencies{stage: func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
		swap, err := stagePackageFiles(home, name, single, files)
		if err != nil {
			return nil, err
		}
		fault := &a4FaultSwap{packageSwap: swap, failRollback: name == "first"}
		if fault.failRollback {
			failed = fault
		}
		return fault, nil
	}, files: func(context.Context, []install.PlannedFile) error { return context.Canceled }}
	result, err := applyUpdate(context.Background(), p.transaction, deps)
	if err == nil || result.Changed || frozenDiagnostic(err).Code != "rollback-failed" || failed == nil {
		t.Fatalf("%+v %v", result, err)
	}
	if !bytes.Equal(readSyncFile(t, filepath.Join(failed.backup, "STYLE.yaml")), []byte("corrupt first")) {
		t.Fatal("failed frozen recovery backup was deleted")
	}
	after := a4State(t, home, config)
	for path, state := range before {
		if !strings.Contains(path, "/packages/first/") && after[path] != state {
			t.Fatal("another rollback was skipped", path)
		}
	}
}
func TestM3A5CapturedInputsAndDetachedConfirmation(t *testing.T) {
	for _, input := range []string{"none", "source", "source-mode", "source-identity", "lock", "lock-mtime", "lock-identity", "approval", "settings", "managed", "index", "explain", "hook", "trust", "parent", "new-file"} {
		t.Run(input, func(t *testing.T) {
			manifest := strings.Replace(nativePackageYAML("first"), "rules:", "settings: [{name: voice, default: Original}]\nrules:", 1)
			s, home, config, origin := a4Setup(t, "codex-user", manifest, false)
			writeSyncFile(t, filepath.Join(home, "packages/first/STYLE.yaml"), "corrupt")
			a5Remove(t, filepath.Join(home, "build/index.json"))
			a5Offline(t, s)
			paths := map[string]string{"source": filepath.Join(origin, "STYLE.yaml"), "source-mode": filepath.Join(origin, "STYLE.yaml"), "source-identity": filepath.Join(origin, "STYLE.yaml"), "lock": filepath.Join(home, "style.lock.yaml"), "lock-mtime": filepath.Join(home, "style.lock.yaml"), "lock-identity": filepath.Join(home, "style.lock.yaml"), "approval": approvalPath(home, "first"), "settings": filepath.Join(home, "settings/first.yaml"), "managed": filepath.Join(home, "packages/first/STYLE.yaml"), "index": filepath.Join(home, "build/index.json"), "explain": filepath.Join(home, "build/explain.json"), "hook": filepath.Join(config, "hooks.json"), "trust": filepath.Join(config, "config.toml")}
			var expected map[string]a4FileState
			result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Confirm: func(preview FrozenResult) (bool, error) {
				// Every mutable preview collection is detached from the committed plan.
				preview.Selected[0] = "changed"
				preview.Changes[0].After.Source.Path = "changed"
				preview.Native[0].Command = "printf UNAPPROVED"
				preview.Contents[0].Snapshot = "changed"
				path := paths[input]
				switch input {
				case "none":
				case "parent":
					if err := os.Chmod(config, 0o755); err != nil {
						t.Fatal(err)
					}
				case "new-file":
					writeSyncFile(t, filepath.Join(home, "packages/new-file"), "external")
				case "source-mode":
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(path, info.Mode().Perm()^0o044); err != nil {
						t.Fatal(err)
					}
				case "lock-mtime":
					if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
						t.Fatal(err)
					}
				case "source-identity", "lock-identity":
					data := readSyncFile(t, path)
					a5Remove(t, path)
					writeSyncFile(t, path, string(data))
				default:
					writeSyncFile(t, path, "changed after capture")
				}
				expected = a4State(t, home, config)
				return true, nil
			}})
			if input == "none" {
				if err != nil || !result.Changed || result.Selected[0] != "first" || result.Native[0].Command == "printf UNAPPROVED" || result.Contents[0].Snapshot != "source" {
					t.Fatalf("%+v %v", result, err)
				}
				return
			}
			if err == nil || result.Diagnostics[len(result.Diagnostics)-1].Code != "installation-changed" || result.Changed {
				t.Fatalf("%+v %v", result, err)
			}
			a4AssertState(t, expected, home, config)
		})
	}
}
func TestM3A5ConfirmationCancellationAndGuard(t *testing.T) {
	for _, phase := range []string{"decline", "confirm-error", "cancel-before", "cancel-confirm", "guard-busy", "preview-unlocked", "stale-guard"} {
		t.Run(phase, func(t *testing.T) {
			s, home, config, _ := a4Setup(t, "codex-user", nativePackageYAML("first"), false)
			a5Remove(t, filepath.Join(home, "build/index.json"))
			a5Offline(t, s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			before := a4State(t, home, config)
			request := FrozenRequest{Home: home, Yes: true}
			var guard *managementGuard
			switch phase {
			case "decline":
				request.Yes = false
			case "confirm-error":
				request.Yes = false
				request.Confirm = func(FrozenResult) (bool, error) { return false, errors.New("CANARY PRIVATE") }
			case "cancel-before":
				cancel()
			case "cancel-confirm":
				request.Yes = false
				request.Confirm = func(FrozenResult) (bool, error) { cancel(); return true, nil }
			case "guard-busy":
				var err error
				guard, err = acquireManagement(ctx, home)
				if err != nil {
					t.Fatal(err)
				}
				defer guard.close()
				before = a4State(t, home, config)
			case "preview-unlocked":
				request.Yes = false
				request.Confirm = func(FrozenResult) (bool, error) {
					g, err := acquireManagement(ctx, home)
					if err != nil {
						t.Fatal(err)
					}
					if err := g.close(); err != nil {
						t.Fatal(err)
					}
					return true, nil
				}
			case "stale-guard":
				writeSyncFile(t, filepath.Join(home, managementName), `{"pid":2147483646,"birth":"missing","nonce":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
				if err := os.Chmod(filepath.Join(home, managementName), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := s.InstallFrozen(ctx, request)
			switch phase {
			case "decline":
				if err != nil || !result.AwaitingConfirmation || result.Changed {
					t.Fatalf("%+v %v", result, err)
				}
			case "preview-unlocked", "stale-guard":
				if err != nil || !result.Changed {
					t.Fatalf("%+v %v", result, err)
				}
				return
			default:
				code := "canceled"
				if phase == "guard-busy" {
					code = "management-busy"
				}
				if phase == "confirm-error" {
					code = "confirmation-failed"
				}
				if err == nil || result.Diagnostics[len(result.Diagnostics)-1].Code != code || strings.Contains(fmt.Sprint(result.Diagnostics), "CANARY") {
					t.Fatalf("%+v %v", result, err)
				}
			}
			a4AssertState(t, before, home, config)
		})
	}
}
func TestM3A5MaintenanceIsolationAndSequence(t *testing.T) {
	s, home, config, _ := a5Batch(t, "claude-user")
	lock := mustA4Lock(t, home)
	lock.Packages[0], lock.Packages[1] = lock.Packages[1], lock.Packages[0]
	a4WriteRawLock(t, home, lock)
	for _, name := range []string{"state/old.json", "stats/old.json", "probe/old.json"} {
		writeSyncFile(t, filepath.Join(home, name), "sentinel "+name)
	}
	a4OldTimes(t, home, config)
	before := a4State(t, home, config)
	check := a5Lock(t, home)
	result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
	if err != nil || !result.Changed || strings.Join(result.Selected, ",") != "first,second" || result.Changes[0].Sequence != 1 || result.Changes[1].Sequence != 2 {
		t.Fatalf("%+v %v", result, err)
	}
	check()
	after := a4State(t, home, config)
	for path, state := range before {
		if strings.Contains(path, "/state/") || strings.Contains(path, "/stats/") || strings.Contains(path, "/probe/") {
			if after[path] != state {
				t.Fatal("frozen maintained runtime data", path)
			}
		}
	}
}
func TestM3A5OwnerAuthorizedRecovery(t *testing.T) {
	for _, route := range a4Routes {
		t.Run(route, func(t *testing.T) {
			s, home, _, origin, foreign := nativeInstallFixture(t, route)
			for path := range foreign {
				a5Remove(t, path)
			}
			// No original mergeable JSON baseline is lost in this fixture. All bound
			// rendered outputs can be recreated with exactly the initial aggregate.
			a4Install(t, s, home, route, origin, false)
			lock := mustA4Lock(t, home)
			adapters, err := s.lockedAdapters(context.Background(), home, lock)
			if err != nil {
				t.Fatal(err)
			}
			paths, err := adapters[0].route.(adapter.ManagementInputs).ManagementPaths()
			if err != nil {
				t.Fatal(err)
			}
			var missing []string
			for _, path := range paths {
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
					missing = append(missing, path)
				}
			}
			if len(missing) == 0 {
				t.Fatal("fixture has no renderer files")
			}
			a5Remove(t, missing...)
			a5Offline(t, s)
			check := a5Lock(t, home)
			result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
			if err != nil || !result.Changed {
				t.Fatalf("paths=%q result=%+v err=%v", missing, result, err)
			}
			check()
			result, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home})
			if err != nil || result.Changed {
				t.Fatalf("repeat=%+v %v", result, err)
			}
		})
	}
}
func TestM3A5BindingAndOwnershipFailures(t *testing.T) {
	for _, scenario := range []string{"surface", "rendered", "trust", "version", "config-root", "missing-route", "missing-provider", "old-layout", "static-hash", "foreign-static", "edited-hook", "foreign-hook", "corrupt-owner", "missing-static", "partial-rendered"} {
		t.Run(scenario, func(t *testing.T) {
			route := "codex-user"
			if scenario == "old-layout" || scenario == "static-hash" || scenario == "foreign-static" || scenario == "missing-static" {
				route = "claude-user"
			}
			s, home, config, _ := a4Setup(t, route, strings.Replace(nativePackageYAML("first"), "rules:", "rules:\n  - {id: static, target: system, text: 'Locked static.'}", 1), false)
			lock := mustA4Lock(t, home)
			provider := nativeProvider(route)
			binding := lock.Harnesses[provider]
			want := "frozen-build-mismatch"
			switch scenario {
			case "surface":
				binding.Surface = hashBytes([]byte("different surface"))
			case "rendered":
				binding.Rendered = hashBytes([]byte("different rendered"))
				a5Remove(t, filepath.Join(config, "hooks.json"))
			case "trust":
				binding.Trust = "managed"
			case "version":
				binding.Version = "99.0.0"
			case "config-root":
				binding.ConfigDir = t.TempDir()
			case "missing-route":
				binding.Route = "missing"
				want = "frozen-binding-mismatch"
			case "missing-provider":
				delete(s.definitions, adapter.ProviderID(provider))
				want = "frozen-binding-mismatch"
			case "old-layout":
				binding.StaticLayout = "claude_md"
				want = "frozen-binding-mismatch"
			case "static-hash":
				for path := range binding.StaticFiles {
					binding.StaticFiles[path] = hashBytes([]byte("wrong"))
				}
				want = "frozen-binding-mismatch"
			case "foreign-static":
				for path := range binding.StaticFiles {
					writeSyncFile(t, filepath.Join(config, path), "foreign static")
				}
				want = "frozen-binding-mismatch"
			case "missing-static":
				for path := range binding.StaticFiles {
					a5Remove(t, filepath.Join(config, path))
				}
				want = "frozen-binding-mismatch"
			case "edited-hook":
				path := filepath.Join(config, "hooks.json")
				writeSyncFile(t, path, strings.ReplaceAll(string(readSyncFile(t, path)), "printf earlier", "printf edited"))
			case "foreign-hook":
				writeSyncFile(t, filepath.Join(config, "hooks.json"), `{"external":true}`)
			case "corrupt-owner":
				paths, err := filepath.Glob(filepath.Join(config, ".agenthooks/style/*.json"))
				if err != nil || len(paths) == 0 {
					t.Fatal(paths, err)
				}
				writeSyncFile(t, paths[0], "corrupt owner")
				want = "frozen-binding-mismatch"
			case "partial-rendered":
				a5Remove(t, filepath.Join(config, "hooks.json"))
			}
			lock.Harnesses[provider] = binding
			a4WriteRawLock(t, home, lock)
			a5Offline(t, s)
			a5Reject(t, s, home, config, want)
		})
	}
}
