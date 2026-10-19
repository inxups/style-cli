package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testgit"
	"github.com/speakeasy-api/agenthooks"
)

func TestM3A3SharedManagementGuard(t *testing.T) {
	for _, command := range []string{"install", "sync", "trust", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			s, home, config, _ := a3Fixture(t, "codex-user")
			ctx := context.Background()
			if _, err := s.Install(ctx, a3Request(home, "codex-user")); err != nil {
				t.Fatal(err)
			}
			if command == "sync" || command == "trust" {
				if err := os.Remove(filepath.Join(home, "build", "explain.json")); err != nil {
					t.Fatal(err)
				}
			}
			guard, err := acquireManagement(ctx, home)
			if err != nil {
				t.Fatal(err)
			}
			before := a3Installed(t, home, config)
			switch command {
			case "install":
				path := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "second"), a6Manifest("second", 50))
				r := a3Request(home, "codex-user")
				r.Source = path
				r.Path = ""
				_, err = s.Install(ctx, r)
			case "sync":
				_, err = s.Sync(ctx, SyncRequest{Home: home})
			case "trust":
				_, err = s.Trust(ctx, TrustRequest{Home: home, Name: "native-test", Yes: true})
			case "uninstall":
				_, err = s.Uninstall(ctx, UninstallRequest{Home: home, All: true, Yes: true})
			}
			if !errors.Is(err, errManagementBusy) || !reflect.DeepEqual(before, a3Installed(t, home, config)) {
				t.Fatalf("%s failed guard: %v", command, err)
			}
			if err := guard.close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestM3A3GuardOwnerRecoveryAndReleaseIdentity(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"live", "gone", "pid-reuse", "unknown", "bad-owner", "malformed", "permissions"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			owner := managementOwner{PID: 123456, Birth: "old-birth", Nonce: strings.Repeat("a", 32)}
			data, _ := json.Marshal(owner)
			if kind == "malformed" {
				data = []byte("invalid")
			}
			if kind == "bad-owner" {
				owner.Birth = ""
				data, _ = json.Marshal(owner)
			}
			mode := os.FileMode(0o600)
			if kind == "permissions" {
				mode = 0o644
			}
			if err := os.WriteFile(filepath.Join(home, managementName), data, mode); err != nil {
				t.Fatal(err)
			}
			lookup := func(pid int) (agenthooks.ProcessIdentity, error) {
				if pid == os.Getpid() {
					return agenthooks.ReadProcessIdentity(pid)
				}
				switch kind {
				case "gone":
					return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessGone
				case "pid-reuse":
					return agenthooks.ProcessIdentity{PID: pid, Birth: "new-birth"}, nil
				case "unknown":
					return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessUnsupported
				default:
					return agenthooks.ProcessIdentity{PID: pid, Birth: "old-birth"}, nil
				}
			}
			guard, err := acquireManagementWith(ctx, home, lookup)
			if kind == "gone" || kind == "pid-reuse" {
				if err != nil {
					t.Fatal(err)
				}
				if err := guard.close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if !errors.Is(err, errManagementBusy) {
					t.Fatal(err)
				}
				if !bytes.Equal(readSyncFile(t, filepath.Join(home, managementName)), data) {
					t.Fatal("busy guard was removed")
				}
			}
			paths, _ := filepath.Glob(filepath.Join(home, ".management-*"))
			if len(paths) != 0 {
				t.Fatalf("guard staging files leaked: %v", paths)
			}
		})
	}
	t.Run("replacement-preserved", func(t *testing.T) {
		home := t.TempDir()
		guard, err := acquireManagement(ctx, home)
		if err != nil {
			t.Fatal(err)
		}
		replacement := filepath.Join(home, "replacement")
		writeSyncFile(t, replacement, "another owner")
		if err := os.Rename(replacement, filepath.Join(home, managementName)); err != nil {
			t.Fatal(err)
		}
		if err := guard.close(); !errors.Is(err, errManagementChanged) {
			t.Fatal(err)
		}
		if string(readSyncFile(t, filepath.Join(home, managementName))) != "another owner" {
			t.Fatal("released another owner's guard")
		}
	})
}

func TestM3A3CommitRejectsStaleInputs(t *testing.T) {
	for _, kind := range []string{"lock", "package", "approval", "settings", "index", "explain", "unchanged-config", "config-mode", "same-bytes-new-inode", "parent-symlink", "new-package"} {
		t.Run(kind, func(t *testing.T) {
			s, home, config, _ := a3Fixture(t, "codex-user")
			ctx := context.Background()
			if _, err := s.Install(ctx, a3Request(home, "codex-user")); err != nil {
				t.Fatal(err)
			}
			path := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "second"), "spec: style/v0.3\nname: second\nversion: 1.0.0\ndescription: second\nrules: [{id: tool, target: tool, tool: shell, at: before, text: second}]\n")
			request := a3Request(home, "codex-user")
			request.Source = path
			request.Path = ""
			request.Yes = false
			var expected map[string]string
			request.Confirm = func(InstallResult) (bool, error) {
				guard, err := acquireManagement(ctx, home)
				if err != nil {
					t.Fatalf("guard held during confirmation: %v", err)
				}
				if err := guard.close(); err != nil {
					t.Fatal(err)
				}
				target := ""
				switch kind {
				case "lock":
					target = filepath.Join(home, "style.lock.yaml")
				case "package":
					target = filepath.Join(home, "packages", "native-test", "notes.txt")
				case "approval":
					target = approvalPath(home, "native-test")
				case "settings":
					target = filepath.Join(home, "settings", "native-test.yaml")
				case "index":
					target = filepath.Join(home, "build", "index.json")
				case "explain":
					target = filepath.Join(home, "build", "explain.json")
				case "unchanged-config", "config-mode", "same-bytes-new-inode":
					target = filepath.Join(config, "config.toml")
				case "new-package":
					target = filepath.Join(home, "packages", "external", "new.md")
				}
				switch kind {
				case "config-mode":
					mode := os.FileMode(0o600)
					if runtime.GOOS == "windows" {
						mode = 0444
					}
					if err := os.Chmod(target, mode); err != nil {
						t.Fatal(err)
					}
				case "same-bytes-new-inode":
					data := readSyncFile(t, target)
					temp := target + ".new"
					if err := os.WriteFile(temp, data, 0o640); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(temp, target); err != nil {
						t.Fatal(err)
					}
				case "parent-symlink":
					moved := config + "-moved"
					if err := os.Rename(config, moved); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(moved, config); err != nil {
						t.Fatal(err)
					}
				default:
					data, _ := os.ReadFile(target)
					writeSyncFile(t, target, string(data)+"\nexternal change\n")
				}
				expected = a3Installed(t, home, config)
				return true, nil
			}
			result, err := s.Install(ctx, request)
			if !errors.Is(err, errManagementChanged) || result.Changed {
				t.Fatalf("%+v %v", result, err)
			}
			if !reflect.DeepEqual(expected, a3Installed(t, home, config)) {
				t.Fatal("stale candidate changed other installation files")
			}
		})
	}
}

func TestM3A3TrustRejectsSourceChangeDuringConfirmation(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(map[bool]string{false: "copy", true: "link"}[link], func(t *testing.T) {
			s, home, path, config := syncFixture(t, link)
			rule := filepath.Join(path, "rules", "shell.md")
			writeSyncFile(t, rule, strings.ReplaceAll(syncRule, "Original", "Approved"))
			before := a3Installed(t, home, config)
			result, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Confirm: func(preview SyncResult) (bool, error) {
				writeSyncFile(t, rule, "unreviewed late edit")
				return true, nil
			}})
			if !errors.Is(err, errManagementChanged) || result.Changed {
				t.Fatalf("%+v %v", result, err)
			}
			delete(before, rule)
			after := a3Installed(t, home, config)
			delete(after, rule)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("stale trust committed approval or configuration")
			}
		})
	}
}

func TestM3A3GitPackageAndFileTransactionRollback(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			s, home, config, f := a3Fixture(t, route)
			ctx := context.Background()
			r := a3Request(home, route)
			if _, err := s.Install(ctx, r); err != nil {
				t.Fatal(err)
			}
			before := a3Installed(t, home, config)
			paths := []string{filepath.Join(home, "build", "index.json"), filepath.Join(home, "build", "explain.json")}
			baseline := map[string][]byte{}
			for _, p := range paths {
				baseline[p] = readSyncFile(t, p)
			}
			f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(nativePackageYAML("second"))}})
			base, cancel := context.WithCancel(ctx)
			defer cancel()
			fault := &a1CancelAfterBuild{Context: base, cancel: cancel, before: baseline}
			result, err := s.Install(fault, r)
			if !errors.Is(err, context.Canceled) || !fault.reached || result.Changed {
				t.Fatalf("%+v %v reached=%t", result, err, fault.reached)
			}
			if !reflect.DeepEqual(before, a3Installed(t, home, config)) {
				t.Fatal("file transaction did not restore previous installation")
			}
			if _, err := os.Stat(filepath.Join(home, "packages", "second")); !os.IsNotExist(err) {
				t.Fatal("failed Git package swap survived")
			}
		})
	}
}

func TestM3A3PackageSwapReportsRecoveryFailure(t *testing.T) {
	root := t.TempDir()
	target, staged := filepath.Join(root, "target"), filepath.Join(root, "staged")
	writeSyncFile(t, target, "original")
	writeSyncFile(t, staged, "candidate")
	swap := swapPackage(target, staged)
	if err := swap.apply(); err != nil {
		t.Fatal(err)
	}
	// Keep the backup, but make its restoration destination's parent missing.
	moved := root + "-moved"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, root)
	if err := swap.rollback(); err == nil || !strings.Contains(err.Error(), "backup retained") {
		t.Fatalf("rollback failure was hidden: %v", err)
	}
	backup := filepath.Join(moved, filepath.Base(swap.backupRoot), "old")
	if string(readSyncFile(t, backup)) != "original" {
		t.Fatal("failed recovery deleted the backup")
	}
}

func TestM3A3NoGitDuringNetworkIndependentCommands(t *testing.T) {
	s, home, config, _ := a3Fixture(t, "codex-user")
	ctx := context.Background()
	if _, err := s.Install(ctx, a3Request(home, "codex-user")); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "called")
	writeSyncFile(t, filepath.Join(bin, "git"), "#!/bin/sh\nprintf called > '"+marker+"'\nexit 99\n")
	if err := os.Chmod(filepath.Join(bin, "git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("STYLE_HOME", home)
	s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
		t.Fatal("offline command used source resolver")
		return source.Snapshot{}, nil
	})
	if _, err := s.List(ctx, ListRequest{Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Doctor(ctx, DoctorRequest{Home: home}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"--style-home=" + home, "--harness-home=" + config, "--harness-version=2.1.315", "agenthooks", "run", "--provider=codex", "--owner=style"}
	payload := `{"session_id":"a3","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"true"},"cwd":"/offline"}`
	if exit := RunWithInput(ctx, args, strings.NewReader(payload), &stdout, &stderr); exit != ExitOK || strings.Contains(stderr.String(), "level=ERROR") || !strings.Contains(stdout.String(), "Be careful.") {
		t.Fatalf("hook exit=%d out=%s err=%s", exit, &stdout, &stderr)
	}
	payloadPath := filepath.Join(t.TempDir(), "payload.json")
	writeSyncFile(t, payloadPath, payload)
	explained, err := s.Explain(ctx, ExplainRequest{Harness: "codex", Event: "PreToolUse", Payload: payloadPath, Index: filepath.Join(home, "build", "index.json")})
	if err != nil || explained.Status != "ok" || explained.Wire == nil {
		t.Fatalf("explain %+v %v", explained, err)
	}
	// The native executable is deliberately absent; this exercises lock 2 and
	// route preflight without contacting any harness or source transport.
	observed, err := s.Probe(ctx, ProbeRequest{Home: home, Harnesses: []string{"codex"}, Timeout: time.Second})
	if err == nil || len(observed.Targets) != 1 || !a2HasCode(observed, "origin-unverified") || observed.Targets[0].CallbackObserved {
		t.Fatalf("probe %+v %v", observed, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("ordinary hook or inspection started Git")
	}
}
