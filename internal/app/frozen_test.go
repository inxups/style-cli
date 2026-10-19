package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testgit"
)

func a5Offline(t *testing.T, s *Service) {
	t.Helper()
	for provider, definition := range s.definitions {
		definition.Detector = a2RejectDetector{t}
		s.definitions[provider] = definition
	}
	s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
		t.Error("frozen invoked acquisition")
		return source.Snapshot{}, errors.New("forbidden acquisition")
	})
}
func a5Remove(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
}
func a5Lock(t *testing.T, home string) func() {
	t.Helper()
	path := filepath.Join(home, "style.lock.yaml")
	before := readSyncFile(t, path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(info, after) || info.Mode() != after.Mode() || !info.ModTime().Equal(after.ModTime()) || !bytes.Equal(before, readSyncFile(t, path)) {
			t.Fatal("frozen replaced or modified lock bytes/mode/mtime/inode", err)
		}
	}
}
func a5Reject(t *testing.T, s *Service, home, config, code string) {
	t.Helper()
	before := a4State(t, home, config)
	check := a5Lock(t, home)
	result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true, AllowReplace: true})
	if err == nil || result.Status != "error" || result.Changed || len(result.Diagnostics) == 0 || result.Diagnostics[len(result.Diagnostics)-1].Code != code {
		t.Fatalf("want=%s result=%+v err=%v", code, result, err)
	}
	a4AssertState(t, before, home, config)
	check()
}
func TestM3A5SelectionAndCLIOptions(t *testing.T) {
	for _, args := range [][]string{{"restore", "source"}, {"restore", "--all"}, {"restore", "--harness=codex"}, {"restore", "--harness", "codex"}, {"restore", "--route=codex=codex-user"}, {"restore", "--set=k=v"}, {"restore", "--link"}, {"restore", "--ref=refs/heads/main"}, {"restore", "--path=pkg"}, {"restore", "--fail-closed=claude-code"}, {"restore", "--lock=other"}, {"restore", "--project"}, {"restore", "--yes", "--yes"}, {"restore", "--frozen"}, {"restore", "--json", "--json"}, {"restore", "--allow-replace", "--allow-replace"}, {"install", "--frozen"}, {"install", "--frozen", "--yes"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			s, home, config, _ := a4Setup(t, "codex-user", a4Manifest("first", "Locked."), false)
			t.Setenv("STYLE_HOME", home)
			a5Offline(t, s)
			before := a4State(t, home, config)
			var out, errOut bytes.Buffer
			if code := RunWithService(context.Background(), s, t.TempDir(), args, &out, &errOut); code != 1 || out.Len() != 0 || errOut.Len() == 0 {
				t.Fatalf("exit=%d out=%s err=%s", code, &out, &errOut)
			}
			a4AssertState(t, before, home, config)
		})
	}
	for _, args := range [][]string{{}, {"--json", "--yes", "--allow-replace"}} {
		if _, _, err := parseFrozenArgs(args); err != nil {
			t.Fatal(err)
		}
	}
}
func TestM3A5NoopAndBuildRecovery(t *testing.T) {
	for _, route := range a4Routes {
		for _, damage := range []string{"none", "index", "explain", "both", "corrupt-build"} {
			t.Run(route+"/"+damage, func(t *testing.T) {
				s, home, config, origin := a4Setup(t, route, nativePackageYAML("first"), false)
				a5Offline(t, s)
				lockPath := filepath.Join(home, "style.lock.yaml")
				raw := append([]byte("# preserve exact lock\n"), readSyncFile(t, lockPath)...)
				writeSyncFile(t, lockPath, string(raw))
				if err := os.Chmod(lockPath, 0o640); err != nil {
					t.Fatal(err)
				}
				a4OldTimes(t, home, config)
				switch damage {
				case "index":
					a5Remove(t, filepath.Join(home, "build/index.json"))
				case "explain":
					a5Remove(t, filepath.Join(home, "build/explain.json"))
				case "both":
					a5Remove(t, filepath.Join(home, "build"))
				case "corrupt-build":
					writeSyncFile(t, filepath.Join(home, "build/index.json"), "broken index")
					writeSyncFile(t, filepath.Join(home, "build/explain.json"), "broken explanation")
				}
				// Upstream drift is irrelevant while the locked managed snapshot exists.
				writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), "changed upstream")
				check := a5Lock(t, home)
				before := a4State(t, home, config)
				preview, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home})
				if err != nil || preview.LockSHA256 != hashBytes(raw) || !preview.LockedSource || preview.Command != "restore" || len(preview.Native) != fixtureNativeCount(2) || len(preview.Reports) != 1 {
					t.Fatalf("preview=%+v %v", preview, err)
				}
				a4AssertState(t, before, home, config)
				check()
				if damage == "none" {
					if preview.Status != "unchanged" || preview.AwaitingConfirmation {
						t.Fatal(preview)
					}
				} else {
					if !preview.AwaitingConfirmation || preview.Status != "confirmation-required" {
						t.Fatal(preview)
					}
				}
				result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
				if err != nil || result.Changed != (damage != "none") {
					t.Fatalf("apply=%+v %v", result, err)
				}
				check()
				before = a4State(t, home, config)
				result, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home})
				if err != nil || result.Status != "unchanged" || result.Changed {
					t.Fatalf("repeat=%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				check()
			})
		}
	}
}
func TestM3A5LocalSnapshotsAndStorageRepair(t *testing.T) {
	for _, kind := range []string{"dir", "file"} {
		for _, candidate := range []string{"managed", "source", "approved", "source-missing", "source-changed", "storage-corrupt", "storage-extra", "storage-kind"} {
			t.Run(kind+"/"+candidate, func(t *testing.T) {
				s, home, config, origin := a4Setup(t, "codex-user", a4Manifest("first", "Locked."), false)
				if kind == "file" {
					if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
						t.Fatal(err)
					}
					origin = filepath.Join(filepath.Dir(origin), "original.style.md")
					writeSyncFile(t, origin, "---\nspec: style/v0.3\nname: first\nversion: 1.0.0\ndescription: single\ntarget: system\n---\nLocked single.\n")
					a4Install(t, s, home, "codex-user", origin, false)
				}
				a5Offline(t, s)
				lock := mustA4Lock(t, home)
				item := lock.Packages[0]
				managed := lockedPackagePath(home, item)
				baseline, _, err := loadApproval(home, item)
				if err != nil {
					t.Fatal(err)
				}
				if candidate != "managed" {
					a5Remove(t, managed)
				}
				switch candidate {
				case "approved", "source-missing":
					a5Remove(t, origin)
				case "source-changed":
					if kind == "dir" {
						writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), a4Manifest("first", "Different upstream."))
					} else {
						writeSyncFile(t, origin, "changed")
					}
				case "storage-corrupt":
					if kind == "dir" {
						writeSyncFile(t, filepath.Join(managed, "STYLE.yaml"), "corrupt")
					} else {
						writeSyncFile(t, managed, "corrupt")
					}
				case "storage-extra":
					if kind == "dir" {
						writeSyncFile(t, filepath.Join(managed, "extra"), "extra")
					} else {
						writeSyncFile(t, managed, "extra")
					}
				case "storage-kind":
					if kind == "dir" {
						writeSyncFile(t, managed, "file instead of directory")
					} else {
						if err := os.MkdirAll(managed, 0o700); err != nil {
							t.Fatal(err)
						}
					}
				}
				before := a4State(t, home, config)
				check := a5Lock(t, home)
				preview, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home})
				if err != nil {
					t.Fatalf("preview=%+v %v", preview, err)
				}
				a4AssertState(t, before, home, config)
				want := "source"
				if candidate == "managed" {
					want = "managed"
				}
				if candidate == "approved" || candidate == "source-missing" || candidate == "source-changed" {
					want = "approved"
				}
				if preview.Contents[0].Snapshot != want {
					t.Fatalf("want=%s contents=%+v", want, preview.Contents)
				}
				result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
				if err != nil || result.Changed != (candidate != "managed") {
					t.Fatalf("%+v %v", result, err)
				}
				files, err := installedSnapshot(home, item)
				if err != nil || !reflect.DeepEqual(files, baseline.Files) {
					t.Fatal("locked bytes not restored", err)
				}
				check()
			})
		}
	}
}
func TestM3A5GitOfflineSnapshotChoices(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		for _, scenario := range []string{"managed-no-cache", "cache", "approved-no-cache", "approved-bad-cache", "approved-cache-symlink", "moved-ref", "unreachable"} {
			t.Run(format+"/"+scenario, func(t *testing.T) {
				s, home, config, _, _ := nativeInstallFixture(t, "codex-user")
				f := testgit.New(t, format, []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(nativePackageYAML("first"))}})
				s.sources = f.Resolver()
				if _, err := s.Install(context.Background(), a3Request(home, "codex-user")); err != nil {
					t.Fatal(err)
				}
				lock := mustA4Lock(t, home)
				managed := lockedPackagePath(home, lock.Packages[0])
				cache := filepath.Join(home, "cache")
				want := "managed"
				switch scenario {
				case "managed-no-cache":
					a5Remove(t, cache)
				case "cache":
					a5Remove(t, managed)
					want = "cache"
				case "approved-no-cache":
					a5Remove(t, cache, managed)
					want = "approved"
				case "approved-bad-cache":
					a5Remove(t, managed)
					dirs, err := filepath.Glob(filepath.Join(cache, "sources/git/v1/*/manifest.json"))
					if err != nil || len(dirs) != 1 {
						t.Fatal(dirs, err)
					}
					writeSyncFile(t, dirs[0], "bad cache")
					want = "approved"
				case "approved-cache-symlink":
					a5Remove(t, cache, managed)
					if err := os.Symlink(t.TempDir(), cache); err != nil {
						t.Fatal(err)
					}
					want = "approved"
				case "moved-ref":
					f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(nativePackageYAML("different"))}})
				case "unreachable":
					a5Remove(t, f.Repo)
				}
				a5Offline(t, s)
				configBefore, err := captureTree(config)
				if err != nil {
					t.Fatal(err)
				}
				cacheBefore, err := captureTree(cache)
				if err != nil {
					t.Fatal(err)
				}
				check := a5Lock(t, home)
				result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
				if err != nil || result.Contents[0].Snapshot != want {
					t.Fatalf("want=%s %+v %v", want, result, err)
				}
				check()
				captured := &managementInputs{trees: map[string]inputTree{cache: cacheBefore, config: configBefore}, parents: map[string]os.FileInfo{}}
				if err := captured.verify(); err != nil {
					t.Fatal("frozen wrote cache", err)
				}
			})
		}
	}
}
func TestM3A5LiveLinksRequireExactTrust(t *testing.T) {
	for _, scenario := range []string{"untrusted", "trusted", "changed", "missing", "symlink", "extra"} {
		t.Run(scenario, func(t *testing.T) {
			s, home, config, origin := a4Setup(t, "codex-user", nativePackageYAML("first"), true)
			if scenario != "untrusted" {
				if _, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Yes: true}); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "changed":
				writeSyncFile(t, filepath.Join(origin, "STYLE.yaml"), nativePackageYAML("first")+"# drift\n")
			case "missing":
				a5Remove(t, origin)
			case "extra":
				writeSyncFile(t, filepath.Join(origin, "extra"), "extra")
			case "symlink":
				file := filepath.Join(origin, "STYLE.yaml")
				data := readSyncFile(t, file)
				a5Remove(t, file)
				outside := filepath.Join(t.TempDir(), "STYLE.yaml")
				writeSyncFile(t, outside, string(data))
				if err := os.Symlink(outside, file); err != nil {
					t.Fatal(err)
				}
			}
			a5Offline(t, s)
			if scenario != "trusted" {
				a5Reject(t, s, home, config, "frozen-source-mismatch")
				return
			}
			a5Remove(t, filepath.Join(home, "build/index.json"))
			check := a5Lock(t, home)
			result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
			if err != nil || result.Contents[0].Snapshot != "link" || result.Contents[0].StorageRepair {
				t.Fatalf("%+v %v", result, err)
			}
			check()
		})
	}
}
func TestM3A5StrictSettingsAndApprovals(t *testing.T) {
	manifest := strings.Replace(a4Manifest("first", "{{settings.voice}}"), "rules:", "settings: [{name: voice, default: Original}, {name: unused, default: Unused}]\nrules:", 1)
	for _, scenario := range []string{"complete", "missing-file", "missing-key", "extra-key", "changed-rendered", "changed-unused", "invalid-yaml", "missing-approval", "corrupt-approval", "no-approval-hash", "changed-native", "legacy-native-omitted"} {
		t.Run(scenario, func(t *testing.T) {
			sourceText := manifest
			if scenario == "changed-native" || scenario == "legacy-native-omitted" {
				requirePOSIXFixture(t, "author native commands require POSIX")
				sourceText = nativePackageYAML("first")
			}
			s, home, config, _ := a4Setup(t, "codex-user", sourceText, false)
			lock := mustA4Lock(t, home)
			settings := filepath.Join(home, "settings/first.yaml")
			want := "frozen-metadata-incomplete"
			switch scenario {
			case "complete":
			case "missing-file":
				a5Remove(t, settings)
			case "missing-key":
				writeSyncFile(t, settings, "voice: Original\n")
			case "extra-key":
				writeSyncFile(t, settings, "voice: Original\nunused: Unused\nextra: CANARY\n")
			case "changed-rendered":
				writeSyncFile(t, settings, "voice: Changed\nunused: Unused\n")
				want = "frozen-build-mismatch"
			case "changed-unused":
				writeSyncFile(t, settings, "voice: Original\nunused: Changed\n")
			case "invalid-yaml":
				writeSyncFile(t, settings, "voice: [invalid\n")
			case "missing-approval":
				a5Remove(t, approvalPath(home, "first"))
			case "corrupt-approval":
				writeSyncFile(t, approvalPath(home, "first"), "corrupt CANARY")
			case "no-approval-hash":
				lock.Packages[0].Approval = ""
				a4WriteRawLock(t, home, lock)
			case "changed-native", "legacy-native-omitted":
				path := approvalPath(home, "first")
				var doc map[string]any
				if err := json.Unmarshal(readSyncFile(t, path), &doc); err != nil {
					t.Fatal(err)
				}
				if scenario == "changed-native" {
					doc["native"].([]any)[0].(map[string]any)["command"] = "printf CANARY"
				} else {
					delete(doc, "native")
				}
				raw, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				writeSyncFile(t, path, string(raw))
				lock.Packages[0].Approval = hashBytes(raw)
				a4WriteRawLock(t, home, lock)
				want = "frozen-build-mismatch"
			}
			a5Offline(t, s)
			if scenario == "complete" || scenario == "changed-unused" {
				before := a4State(t, home, config)
				result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
				if err != nil || result.Changed {
					t.Fatalf("%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				return
			}
			a5Reject(t, s, home, config, want)
		})
	}
}
func TestM3A5LockMetadataAndCompatibility(t *testing.T) {
	for _, scenario := range []string{"format1", "format2-local", "implicit", "zero", "unknown", "empty", "duplicate-name", "duplicate-seq", "invalid-path", "missing-binding", "no-surface", "no-rendered", "no-config", "no-version", "no-trust", "invalid-source"} {
		t.Run(scenario, func(t *testing.T) {
			s, home, config, _ := a4Setup(t, "codex-user", a4Manifest("first", "Locked."), false)
			lock := mustA4Lock(t, home)
			want := "frozen-metadata-incomplete"
			switch scenario {
			case "format1":
			case "format2-local":
				lock.Format = 2
			case "implicit", "zero":
				lock.Format = 0
			case "unknown":
				lock.Format = 99
			case "empty":
				lock.Packages = nil
			case "duplicate-name":
				lock.Packages = append(lock.Packages, lock.Packages[0])
				want = "lock-invalid"
			case "duplicate-seq":
				second := lock.Packages[0]
				second.Name = "second"
				lock.Packages = append(lock.Packages, second)
				want = "lock-invalid"
			case "invalid-path":
				lock.Packages[0].Files = map[string]string{"../escape": hashBytes([]byte("x"))}
				want = "lock-invalid"
			case "missing-binding":
				lock.Harnesses = nil
			case "no-surface", "no-rendered", "no-config", "no-version", "no-trust":
				v := lock.Harnesses["codex"]
				switch scenario {
				case "no-surface":
					v.Surface = ""
				case "no-rendered":
					v.Rendered = ""
				case "no-config":
					v.ConfigDir = ""
				case "no-version":
					v.Version = ""
				case "no-trust":
					v.Trust = ""
				}
				lock.Harnesses["codex"] = v
			case "invalid-source":
				lock.Packages[0].Source.Kind = "invalid"
				want = "lock-invalid"
			}
			a4WriteRawLock(t, home, lock)
			if scenario == "implicit" {
				path := filepath.Join(home, "style.lock.yaml")
				s := strings.Replace(string(readSyncFile(t, path)), "format: 0\n", "", 1)
				writeSyncFile(t, path, s)
			}
			a5Offline(t, s)
			if scenario == "format1" || scenario == "format2-local" {
				check := a5Lock(t, home)
				result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home})
				if err != nil || result.Changed {
					t.Fatalf("%+v %v", result, err)
				}
				check()
			} else {
				a5Reject(t, s, home, config, want)
			}
		})
	}
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-lock", true: "empty-new-home"}[missing], func(t *testing.T) {
			s, home, config, _, _ := nativeInstallFixture(t, "codex-user")
			if missing {
				home = filepath.Join(t.TempDir(), "new-home")
			}
			a5Offline(t, s)
			before := a4State(t, home, config)
			result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
			if err == nil || result.Diagnostics[0].Code != "frozen-metadata-incomplete" {
				t.Fatalf("%+v %v", result, err)
			}
			a4AssertState(t, before, home, config)
		})
	}
}
func TestM3A5PrivateInputAndStorageSymlinks(t *testing.T) {
	for _, input := range []string{"lock", "approved", "approval-file", "settings", "settings-file", "build", "index", "explain", "packages", "package", "package-file"} {
		t.Run(input, func(t *testing.T) {
			s, home, config, _ := a4Setup(t, "codex-user", strings.Replace(a4Manifest("first", "{{settings.voice}}"), "rules:", "settings: [{name: voice, default: Original}]\nrules:", 1), false)
			names := map[string]string{"lock": "style.lock.yaml", "approved": "approved", "approval-file": "approved/first.json", "settings": "settings", "settings-file": "settings/first.yaml", "build": "build", "index": "build/index.json", "explain": "build/explain.json", "packages": "packages", "package": "packages/first", "package-file": "packages/first/STYLE.yaml"}
			path := filepath.Join(home, names[input])
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.Rename(path, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			a5Offline(t, s)
			before := a4State(t, home, config)
			result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true})
			want := "installation-changed"
			if strings.HasPrefix(input, "package") {
				want = "frozen-source-mismatch"
			}
			if err == nil || result.Diagnostics[0].Code != want {
				t.Fatalf("%+v %v", result, err)
			}
			a4AssertState(t, before, home, config)
		})
	}
}
