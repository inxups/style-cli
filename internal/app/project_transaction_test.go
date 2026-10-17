package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks/install"
)

func a6ChangedBatch(t *testing.T) (*Service, string, string, []string) {
	t.Helper()
	s, base, root, sources := a6Batch(t)
	first := filepath.Join(sources[0], "STYLE.yaml")
	writeSyncFile(t, first, strings.ReplaceAll(string(readSyncFile(t, first)), "printf earlier", "printf updated"))
	second := filepath.Join(sources[1], "STYLE.yaml")
	writeSyncFile(t, second, strings.ReplaceAll(string(readSyncFile(t, second)), "Use {{settings.voice}}.", "Updated {{settings.voice}}."))
	for path := range a4State(t, base, root) {
		if strings.Contains(path, "/cache/") {
			continue
		}
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	// Keep executable launcher permission, which is part of the normal binding.
	if err := os.Chmod(filepath.Join(root, projectLauncherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	a4OldTimes(t, base, root)
	return s, base, root, sources
}
func TestM3A6EachFileFailureRestoresAllRoots(t *testing.T) {
	s, _, _, _ := a6ChangedBatch(t)
	prepared, err := s.prepareUpdate(context.Background(), UpdateRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	count := len(prepared.files) + 1
	seenRoots := map[string]bool{}
	for _, file := range prepared.files {
		if strings.Contains(file.Path, "/projects/") {
			seenRoots["private"] = true
		}
		if strings.Contains(file.Path, "/.claude/") || strings.Contains(file.Path, "/.codex/") {
			seenRoots["project"] = true
		}
		if filepath.Base(file.Path) == "config.toml" {
			seenRoots["user-trust"] = true
		}
	}
	if len(seenRoots) != 3 {
		t.Fatal("incomplete cross-root plan", seenRoots)
	}
	for index := 0; index < count; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			s, base, root, _ := a6ChangedBatch(t)
			before := a4State(t, filepath.Dir(base), root)
			p, err := s.prepareUpdate(context.Background(), UpdateRequest{All: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(p.files)+1 != count {
				t.Fatal("plan changed")
			}
			var fault *a4CancelAt
			deps := updateDependencies{files: func(ctx context.Context, files []install.PlannedFile) error {
				all := append(append([]install.PlannedFile(nil), files...), install.PlannedFile{Root: base, Path: filepath.Join(base, ".a6-sentinel"), After: []byte("never"), Mode: 0o600})
				fault = &a4CancelAt{Context: ctx, at: 2*len(all) + index + 1}
				return install.ApplyFiles(fault, all)
			}}
			result, err := applyUpdate(context.Background(), p, deps)
			if !errors.Is(err, context.Canceled) || result.Changed || fault == nil || !fault.reached {
				t.Fatal(result, err, fault)
			}
			a4AssertState(t, before, filepath.Dir(base), root)
		})
	}
}
func TestM3A6EachPackageFailureAndRecovery(t *testing.T) {
	for _, stage := range []string{"stage-project-rules", "stage-second", "apply-project-rules", "apply-second", "rollback"} {
		t.Run(stage, func(t *testing.T) {
			s, base, root, _ := a6ChangedBatch(t)
			before := a4State(t, filepath.Dir(base), root)
			p, err := s.prepareUpdate(context.Background(), UpdateRequest{All: true})
			if err != nil {
				t.Fatal(err)
			}
			var failed *a4FaultSwap
			deps := updateDependencies{stage: func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
				if stage == "stage-"+name {
					return nil, errors.New("injected package stage failure")
				}
				swap, err := stagePackageFiles(home, name, single, files)
				if err != nil {
					return nil, err
				}
				f := &a4FaultSwap{packageSwap: swap, failApply: stage == "apply-"+name, failRollback: stage == "rollback" && name == "project-rules"}
				if f.failRollback {
					failed = f
				}
				return f, nil
			}}
			if stage == "rollback" {
				deps.files = func(context.Context, []install.PlannedFile) error { return context.Canceled }
			}
			result, err := applyUpdate(context.Background(), p, deps)
			if err == nil || result.Changed {
				t.Fatal(result, err)
			}
			if stage == "rollback" {
				if updateDiagnostic(err).Code != "rollback-failed" || failed == nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(readSyncFile(t, filepath.Join(failed.backup, "STYLE.yaml"))), "printf earlier") {
					t.Fatal("lost recovery backup")
				}
				return
			}
			a4AssertState(t, before, filepath.Dir(base), root)
		})
	}
}
func TestM3A6FreshBatchCancellationIsAtomic(t *testing.T) {
	checks := 0
	for at := 1; at <= 512; at++ {
		reached := false
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			s, base, root, first := a6Setup(t)
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(root), "extra-source"), a4Manifest("second", "Extra."))
			scoped := a6Scope(t, s, root, base)
			before := a4State(t, filepath.Dir(base), root)
			ctx := &a4CancelAt{Context: context.Background(), at: at}
			result, err := scoped.Install(ctx, InstallRequest{Source: first, ProjectSources: []string{second}, Yes: true})
			checks = ctx.checks
			reached = ctx.reached
			if reached {
				if err == nil || result.Changed {
					t.Fatal(result, err, ctx)
				}
				a4AssertState(t, before, filepath.Dir(base), root)
			} else if err != nil || !result.Changed {
				t.Fatal(result, err)
			}
		})
		if !reached {
			t.Logf("all %d context checkpoints covered", checks)
			return
		}
	}
	t.Fatal("checkpoint enumeration exceeded bound")
}
func TestM3A6ConfirmationRejectsCrossScopeDrift(t *testing.T) {
	for _, input := range []string{"declaration", "binding", "registry", "user-lock", "other-project-lock", "launcher", "user-trust", "project-hook", "source"} {
		t.Run(input, func(t *testing.T) {
			s, base, root, sources := a6ChangedBatch(t)
			other := filepath.Join(filepath.Dir(root), "other-project")
			if err := os.Mkdir(other, 0o755); err != nil {
				t.Fatal(err)
			}
			b := a6Scope(t, s, other, base)
			a6Install(t, b, sources[1])
			paths := map[string]string{"declaration": filepath.Join(root, projectDeclarationPath), "binding": filepath.Join(s.project.Home, "binding.json"), "registry": filepath.Join(base, "project-registry.json"), "user-lock": filepath.Join(base, "style.lock.yaml"), "other-project-lock": filepath.Join(b.project.Home, "style.lock.yaml"), "launcher": filepath.Join(root, projectLauncherPath), "user-trust": filepath.Join(os.Getenv("CODEX_HOME"), "config.toml"), "project-hook": filepath.Join(root, ".codex/hooks.json"), "source": filepath.Join(sources[0], "STYLE.yaml")}
			var expected map[string]a4FileState
			request := UpdateRequest{All: true, Confirm: func(UpdateResult) (bool, error) {
				p := paths[input]
				data, _ := os.ReadFile(p)
				writeSyncFile(t, p, string(data)+"\nchanged during confirmation\n")
				expected = a4State(t, filepath.Dir(base), root)
				return true, nil
			}}
			result, err := s.Update(context.Background(), request)
			if !errors.Is(err, errManagementChanged) || result.Changed || expected == nil {
				t.Fatal(result, err)
			}
			a4AssertState(t, expected, filepath.Dir(base), root)
		})
	}
}
func TestM3A6UserAndProjectWritersShareGuard(t *testing.T) {
	for _, kind := range []string{"install", "update", "sync", "trust", "uninstall", "user-install"} {
		t.Run(kind, func(t *testing.T) {
			s, base, root, sources := a6ChangedBatch(t)
			guard, err := acquireManagement(context.Background(), s.project.Base)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(filepath.Join(s.project.Home, "build/explain.json")); err != nil {
				t.Fatal(err)
			}
			third := writeInstallPackage(t, filepath.Join(filepath.Dir(root), "third-source"), a4Manifest("third", "Third."))
			before := a4State(t, filepath.Dir(base), root)
			switch kind {
			case "install":
				_, err = s.Install(context.Background(), InstallRequest{Source: third, Yes: true})
			case "update":
				_, err = s.Update(context.Background(), UpdateRequest{All: true, Yes: true})
			case "sync":
				_, err = s.Sync(context.Background(), SyncRequest{})
			case "trust":
				_, err = s.Trust(context.Background(), TrustRequest{Name: "project-rules", Yes: true})
			case "uninstall":
				_, err = s.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true})
			case "user-install":
				user := *s
				user.project = nil
				user.definitions = mapUserDefinitions(t, s)
				_, err = user.Install(context.Background(), InstallRequest{Source: third, Yes: true})
			}
			_ = sources
			if !errors.Is(err, errManagementBusy) {
				t.Fatal(kind, err)
			}
			a4AssertState(t, before, filepath.Dir(base), root)
			if err = guard.close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mapUserDefinitions(t *testing.T, s *Service) map[adapter.ProviderID]adapter.Definition {
	t.Helper()
	result := map[adapter.ProviderID]adapter.Definition{}
	for id, d := range s.definitions {
		inst, _, err := d.Detect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if id == codex.ProviderName {
			d = codex.DefinitionForInstallation(inst)
		} else {
			d = claude.DefinitionForInstallation(inst)
		}
		d.Detector = fixedDetector{inst}
		result[id] = d
	}
	return result
}
