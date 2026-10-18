package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/tokens"

	"github.com/inxups/style-cli/internal/source"
	"github.com/inxups/style-cli/internal/testgit"
	"gopkg.in/yaml.v3"
)

type a3Acquirer func(context.Context, source.Request) (source.Snapshot, error)

func (f a3Acquirer) Resolve(ctx context.Context, r source.Request) (source.Snapshot, error) {
	return f(ctx, r)
}
func a3Fixture(t *testing.T, route string) (*Service, string, string, *testgit.Fixture) {
	t.Helper()
	s, home, config, _, _ := nativeInstallFixture(t, route)
	f := testgit.New(t, "sha1", []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(nativePackageYAML("native-test"))}, {Path: "pkg/notes.txt", Mode: "100755", Data: []byte("ordinary data\n")}})
	s.sources = f.Resolver()
	return s, home, config, f
}
func a3Request(home, route string) InstallRequest {
	p := nativeProvider(route)
	return InstallRequest{Home: home, Source: testgit.URL, Path: "pkg", Harnesses: []string{p}, Routes: map[string]string{p: route}, Yes: true}
}
func a3Installed(t *testing.T, home, config string) map[string]string {
	t.Helper()
	all := map[string]string{}
	for _, root := range []string{home, config} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
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
			info, err := d.Info()
			if err != nil {
				return err
			}
			var data []byte
			if d.Type()&os.ModeSymlink != 0 {
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
			all[p] = fmt.Sprintf("%v|%s", info.Mode(), data)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return all
}

func TestM3A3GitCLIAndLifecycle(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			s, home, config, f := a3Fixture(t, route)
			t.Setenv("STYLE_HOME", home)
			ctx := context.Background()
			for _, command := range []string{"validate", "plan", "install"} {
				t.Run("preview-"+command, func(t *testing.T) {
					before := a3Installed(t, home, config)
					args := []string{command, testgit.URL, "--ref=refs/heads/main", "--path", "pkg", "--json"}
					if command != "validate" {
						args = append(args, "--harness", nativeProvider(route))
					}
					if command == "install" {
						args = append(args, "--route", nativeProvider(route)+"="+route)
					}
					var out, errOut bytes.Buffer
					exit := RunWithService(ctx, s, t.TempDir(), args, &out, &errOut)
					want := ExitOK
					if command == "install" {
						want = ExitInvalid
					}
					if exit != want {
						t.Fatalf("exit=%d out=%s err=%s", exit, &out, &errOut)
					}
					var result map[string]any
					if err := json.Unmarshal(out.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					metadata, ok := result["source"].(map[string]any)
					if !ok || metadata["git"].(map[string]any)["commit"] != f.Commit {
						t.Fatalf("missing provenance: %s", out.String())
					}
					if !reflect.DeepEqual(before, a3Installed(t, home, config)) {
						t.Fatal("preview changed installation")
					}
				})
			}
			request := a3Request(home, route)
			result, err := s.Install(ctx, request)
			if err != nil || !result.Changed || result.Source == nil || len(result.Native) != fixtureNativeCount(2) {
				t.Fatalf("%+v %v", result, err)
			}
			lock := mustLoadNativeLock(t, home)
			item := lock.Packages[0]
			if lock.Format != 2 || item.Source.Git == nil || item.Source.Git.Commit != f.Commit || item.Source.Path != "pkg" || item.Source.Kind != "dir" || item.Link {
				t.Fatalf("%+v", lock)
			}
			if item.Source.Git.ContentSHA256 != packageLockHash(item.Files) {
				t.Fatal("aggregate differs from existing lock algorithm")
			}
			info, err := os.Stat(filepath.Join(home, "packages", "native-test", "notes.txt"))
			if err != nil || info.Mode().Perm() != fixtureMode(0o600, false) {
				t.Fatal("Git executable bit was retained")
			}
			before := a3Installed(t, home, config)
			again, err := s.Install(ctx, request)
			if err != nil || again.Changed || !again.Source.CacheHit || !reflect.DeepEqual(before, a3Installed(t, home, config)) {
				t.Fatalf("idempotent reinstall: %+v %v", again, err)
			}
			s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
				t.Fatal("read-only/lifecycle operation reacquired Git")
				return source.Snapshot{}, nil
			})
			listed, err := s.List(ctx, ListRequest{Home: home})
			if err != nil || listed.Packages[0].Git == nil || !listed.Packages[0].HashMatches {
				t.Fatalf("%+v %v", listed, err)
			}
			var text bytes.Buffer
			writeListText(&text, listed.Packages)
			if !strings.Contains(text.String(), f.Commit) {
				t.Fatal("text omitted Git identity")
			}
			if _, err := s.Doctor(ctx, DoctorRequest{Home: home}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Sync(ctx, SyncRequest{Home: home}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Trust(ctx, TrustRequest{Home: home, Name: item.Name, Yes: true}); err != nil {
				t.Fatal(err)
			}
			current := mustLoadNativeLock(t, home)
			if current.Format != 2 || *current.Packages[0].Source.Git != *item.Source.Git {
				t.Fatal("lifecycle lost Git identity")
			}
			if _, err := s.Uninstall(ctx, UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(home, "style.lock.yaml")); !os.IsNotExist(err) {
				t.Fatal("last uninstall retained lock")
			}
		})
	}
}

func TestM3A3SingleFileAndLockUpgrade(t *testing.T) {
	s, home, config, f := a3Fixture(t, "codex-user")
	ctx := context.Background()
	local := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "local"), "spec: style/v0.3\nname: local\nversion: 1.0.0\ndescription: local\nrules: [{id: base, target: system, text: local}]\n")
	request := a3Request(home, "codex-user")
	request.Source = local
	request.Path = ""
	if _, err := s.Install(ctx, request); err != nil {
		t.Fatal(err)
	}
	if mustLoadNativeLock(t, home).Format != 1 {
		t.Fatal("local lock upgraded early")
	}
	f.MakeCommit([]testgit.Entry{{Path: "nested/reviewer.style.md", Data: []byte("---\nspec: style/v0.3\nname: single\nversion: 1.0.0\ndescription: single\ntarget: system\n---\nSingle file rule.\n")}})
	request.Source = testgit.URL
	request.Path = "nested/reviewer.style.md"
	if _, err := s.Install(ctx, request); err != nil {
		t.Fatal(err)
	}
	lock := mustLoadNativeLock(t, home)
	item, ok := findLockedPackage(lock, "single")
	if !ok || lock.Format != 2 || item.Source.Kind != "file" || len(item.Files) != 1 || item.Files["reviewer.style.md"] == "" {
		t.Fatalf("%+v", lock)
	}
	if _, err := os.Stat(filepath.Join(home, "packages", "single.style.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(ctx, SyncRequest{Home: home}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Uninstall(ctx, UninstallRequest{Home: home, Name: "single", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if lock := mustLoadNativeLock(t, home); lock.Format != 2 || len(lock.Packages) != 1 || lock.Packages[0].Name != "local" {
		t.Fatal("nonempty format 2 downgraded")
	}
	if _, err := s.Uninstall(ctx, UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	_ = config
}

func TestM3A3GitDriftAndProvenanceForgery(t *testing.T) {
	for _, scenario := range []string{"bytes", "missing", "symlink", "commit-text", "object-text"} {
		t.Run(scenario, func(t *testing.T) {
			s, home, config, _ := a3Fixture(t, "codex-user")
			ctx := context.Background()
			if _, err := s.Install(ctx, a3Request(home, "codex-user")); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, "packages", "native-test", "STYLE.yaml")
			switch scenario {
			case "bytes":
				writeSyncFile(t, path, strings.ReplaceAll(nativePackageYAML("native-test"), "printf earlier", "printf unapproved"))
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(filepath.Dir(home), "outside"), path); err != nil {
					t.Fatal(err)
				}
			default:
				lock := mustLoadNativeLock(t, home)
				git := lock.Packages[0].Source.Git
				if scenario == "commit-text" {
					git.Commit = strings.Repeat("a", 40)
				} else {
					git.Object = strings.Repeat("b", 40)
				}
				if err := writeLock(home, lock); err != nil {
					t.Fatal(err)
				}
			}
			before := a3Installed(t, home, config)
			s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
				t.Fatal("drift check invoked Git")
				return source.Snapshot{}, nil
			})
			for _, command := range []string{"sync", "trust"} {
				var err error
				if command == "sync" {
					_, err = s.Sync(ctx, SyncRequest{Home: home})
				} else {
					_, err = s.Trust(ctx, TrustRequest{Home: home, Name: "native-test", Yes: true})
				}
				if err == nil {
					t.Fatalf("%s accepted changed Git provenance", command)
				}
				if scenario == "bytes" || scenario == "missing" || scenario == "symlink" {
					if source.Code(err) != "git-snapshot-changed" {
						t.Fatal(err)
					}
				}
			}
			listed, err := s.List(ctx, ListRequest{Home: home})
			if err != nil || listed.Packages[0].Git == nil || listed.Packages[0].HashMatches && scenario != "commit-text" && scenario != "object-text" {
				t.Fatalf("%+v %v", listed, err)
			}
			if scenario == "bytes" {
				doctor, err := s.Doctor(ctx, DoctorRequest{Home: home})
				if err != nil || !doctorHas(doctor, "git-snapshot-changed") {
					t.Fatalf("%+v %v", doctor, err)
				}
			}
			if !reflect.DeepEqual(before, a3Installed(t, home, config)) {
				t.Fatal("failed admission mutated installation")
			}
		})
	}
}

func TestM3A3GitReinstallIdentityAndCandidateCapture(t *testing.T) {
	s, home, config, f := a3Fixture(t, "codex-user")
	ctx := context.Background()
	request := a3Request(home, "codex-user")
	if _, err := s.Install(ctx, request); err != nil {
		t.Fatal(err)
	}
	before := a3Installed(t, home, config)
	first := f.Commit
	for _, scenario := range []string{"selector", "revision-only", "route", "local-conversion"} {
		t.Run(scenario, func(t *testing.T) {
			f.Run(nil, "update-ref", "refs/heads/main", first)
			r := request
			switch scenario {
			case "selector":
				r.Ref = f.Commit
			case "revision-only":
				f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(nativePackageYAML("native-test"))}, {Path: "pkg/notes.txt", Mode: "100755", Data: []byte("ordinary data\n")}})
			case "route":
				requireFixtureRoute(t, "codex-plugin")
				r.Ref = ""
				r.Routes = map[string]string{"codex": "codex-plugin"}
			case "local-conversion":
				r.Source = filepath.Join(home, "packages", "native-test")
				r.Path = ""
			}
			_, err := s.Install(ctx, r)
			var conflict *ConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("%s: %v", scenario, err)
			}
			if !reflect.DeepEqual(before, a3Installed(t, home, config)) {
				t.Fatal("conflicting reinstall wrote files")
			}
		})
	}
	t.Run("confirmation-does-not-resolve-again", func(t *testing.T) {
		s, home, _, f := a3Fixture(t, "codex-user")
		first := f.Commit
		r := a3Request(home, "codex-user")
		r.Yes = false
		r.Confirm = func(preview InstallResult) (bool, error) {
			if preview.Source.Git.Commit != first {
				t.Fatal("wrong preview")
			}
			preview.Source.Git.Commit = "callback mutation"
			f.MakeCommit([]testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte("invalid later commit\n")}})
			return true, nil
		}
		result, err := s.Install(ctx, r)
		if err != nil || result.Source.Git.Commit != first || mustLoadNativeLock(t, home).Packages[0].Source.Git.Commit != first {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

func TestM3A3LockTwoStrictDecoding(t *testing.T) {
	s, home, _, _ := a3Fixture(t, "codex-user")
	if _, err := s.Install(context.Background(), a3Request(home, "codex-user")); err != nil {
		t.Fatal(err)
	}
	data := readSyncFile(t, filepath.Join(home, "style.lock.yaml"))
	for _, scenario := range []string{"unknown-format", "format-one-git", "unknown-git-field", "unknown-package-field", "bad-url", "bad-ref", "bad-commit", "bad-object-format", "bad-content-hash", "bad-file-hash", "link", "kind", "path", "missing-approval", "duplicate-package", "second-document"} {
		t.Run(scenario, func(t *testing.T) {
			var lock lockDocument
			if err := yaml.Unmarshal(data, &lock); err != nil {
				t.Fatal(err)
			}
			item := &lock.Packages[0]
			extra := ""
			switch scenario {
			case "unknown-format":
				lock.Format = 3
			case "format-one-git":
				lock.Format = 1
			case "unknown-git-field":
				extra = "git"
			case "unknown-package-field":
				extra = "package"
			case "bad-url":
				item.Source.Git.URL = "https://secret:CANARY@host/repo"
			case "bad-ref":
				item.Source.Git.Ref = "main"
			case "bad-commit":
				item.Source.Git.Commit = "abcd"
			case "bad-object-format":
				item.Source.Git.ObjectFormat = "sha512"
			case "bad-content-hash":
				item.Source.Git.ContentSHA256 = "sha256:" + strings.Repeat("a", 64)
			case "bad-file-hash":
				item.Files["STYLE.yaml"] = "sha1:123"
			case "link":
				item.Link = true
				item.Source.Link = true
			case "kind":
				item.Source.Kind = "git"
			case "path":
				item.Source.Path = "../pkg"
			case "missing-approval":
				item.Approval = ""
			case "duplicate-package":
				lock.Packages = append(lock.Packages, *item)
			case "second-document":
				extra = "document"
			}
			mutated, _ := yaml.Marshal(lock)
			switch extra {
			case "git":
				mutated = bytes.Replace(mutated, []byte("git:\n"), []byte("git:\n          unexpected: true\n"), 1)
			case "package":
				mutated = append(mutated, []byte("unexpected: true\n")...)
			case "document":
				mutated = append(mutated, []byte("---\nformat: 2\n")...)
			}
			if _, err := decodeLock(mutated); err == nil {
				t.Fatalf("accepted %s: %s", scenario, mutated)
			}
		})
	}
	t.Run("format-one-null-git", func(t *testing.T) {
		_, err := decodeLock([]byte("format: 1\npackages: [{name: x, source: {kind: dir, path: /tmp/x, git: null}}]\n"))
		if err == nil {
			t.Fatal("format 1 silently accepted a Git field")
		}
	})
}

func TestM3A3FlagsFailBeforeAcquisition(t *testing.T) {
	s := testService(t)
	s.sources = a3Acquirer(func(context.Context, source.Request) (source.Snapshot, error) {
		t.Fatal("invalid option combination acquired Git")
		return source.Snapshot{}, nil
	})
	for _, command := range []string{"validate", "plan", "install"} {
		for _, flags := range [][]string{{"--ref"}, {"--path="}, {"--ref=refs/heads/main", "--ref=refs/heads/other"}, {"--path=pkg", "--path=other"}} {
			t.Run(command+fmt.Sprint(flags), func(t *testing.T) {
				var out, errOut bytes.Buffer
				args := append([]string{command, testgit.URL}, flags...)
				if exit := RunWithService(context.Background(), s, t.TempDir(), args, &out, &errOut); exit != ExitInvalid {
					t.Fatalf("%d %s %s", exit, &out, &errOut)
				}
			})
		}
	}
	for _, command := range []string{"validate", "plan", "install"} {
		t.Run(command+"-local-ref", func(t *testing.T) {
			var out, errOut bytes.Buffer
			exit := RunWithService(context.Background(), s, t.TempDir(), []string{command, "local", "--ref=refs/heads/main", "--json"}, &out, &errOut)
			if exit != ExitInvalid || !strings.Contains(out.String()+errOut.String(), "source-invalid") {
				t.Fatalf("%d %s %s", exit, &out, &errOut)
			}
		})
	}
	if _, err := s.Install(context.Background(), InstallRequest{Source: testgit.URL, Link: true, Home: t.TempDir()}); source.Code(err) != "source-invalid" {
		t.Fatal(err)
	}
}

func TestM3A3GitAdmissionUsesExistingGates(t *testing.T) {
	header := "spec: style/v0.3\nname: guarded\nversion: 1.0.0\ndescription: guarded\n"
	base := "rules: [{id: base, target: system, text: Long static instruction for all reviews.}]\n"
	for _, kind := range []string{"schema", "requires-runtime", "requires-harness", "unknown-harness", "invalid-native", "budget", "input-ceiling", "replace", "missing-setting", "invalid-template"} {
		t.Run(kind, func(t *testing.T) {
			s, home, config, _ := a3Fixture(t, "codex-user")
			manifest := header + base
			switch kind {
			case "schema":
				manifest = strings.ReplaceAll(manifest, "version: 1.0.0", "version: invalid")
			case "requires-runtime":
				manifest = header + "requires: {runtime: '>=9.0.0'}\n" + base
			case "requires-harness":
				manifest = header + "requires: {harness: {codex: '>=99.0.0'}}\n" + base
			case "unknown-harness":
				manifest = header + "requires: {harness: {missing: '>=1.0.0'}}\n" + base
			case "invalid-native":
				manifest = header + "rules: []\nnative_hooks:\n  - {id: raw, codex: {event: UnknownEvent, command: unsafe}}\n"
			case "budget":
				manifest = header + "budget: {per_rule: 1, per_event: 1, always_on: 1, on: error}\n" + base
			case "input-ceiling":
				manifest = header + "rules: [{id: tool, target: tool, tool: shell, text: dynamically injected instruction}]\n"
				definition := s.definitions["codex"]
				definition.Limits.Input = tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}
				s.definitions["codex"] = definition
			case "replace":
				manifest = strings.ReplaceAll(manifest, "target: system", "target: system, mode: replace")
			case "missing-setting":
				manifest = header + "settings: [{name: required}]\n" + base
			case "invalid-template":
				manifest = strings.ReplaceAll(manifest, "Long static instruction for all reviews.", "{{settings.missing}}")
			}
			f := testgit.New(t, "sha1", []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(manifest)}})
			s.sources = f.Resolver()
			before := a3Installed(t, home, config)
			result, err := s.Install(context.Background(), a3Request(home, "codex-user"))
			if err == nil && !(kind == "requires-harness" && len(result.Diagnostics) > 0) {
				t.Fatalf("%s accepted invalid candidate: %+v %v", kind, result, err)
			}
			if result.Changed || !reflect.DeepEqual(before, a3Installed(t, home, config)) {
				t.Fatal("Git admission failure wrote installation state")
			}
		})
	}
}

func TestM3A3GitSettingsArePartOfReinstallIdentity(t *testing.T) {
	s, home, config, _ := a3Fixture(t, "codex-user")
	f := testgit.New(t, "sha1", []testgit.Entry{{Path: "pkg/STYLE.yaml", Data: []byte(syncManifest)}, {Path: "pkg/rules/shell.md", Data: []byte(syncRule)}})
	s.sources = f.Resolver()
	request := a3Request(home, "codex-user")
	request.Settings = map[string]string{"voice": "direct"}
	if _, err := s.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if result, err := s.Install(context.Background(), request); err != nil || result.Changed {
		t.Fatalf("%+v %v", result, err)
	}
	before := a3Installed(t, home, config)
	request.Settings = map[string]string{"voice": "polite"}
	if _, err := s.Install(context.Background(), request); err == nil {
		t.Fatal("different settings treated as idempotent")
	}
	if !reflect.DeepEqual(before, a3Installed(t, home, config)) {
		t.Fatal("conflicting settings were written")
	}
}

// The optional destination exports only a test-generated installation for the
// separate real old-binary gate. No production command reads this variable.
func TestM3A3ExportFormatTwoFixture(t *testing.T) {
	s, home, config, _ := a3Fixture(t, "codex-user")
	if _, err := s.Install(context.Background(), a3Request(home, "codex-user")); err != nil {
		t.Fatal(err)
	}
	if mustLoadNativeLock(t, home).Format != 2 {
		t.Fatal("export fixture is not format 2")
	}
	destination := os.Getenv("STYLE_M3_A3_EXPORT")
	if destination == "" {
		return
	}
	for _, pair := range [][2]string{{home, "home"}, {config, "config"}} {
		err := filepath.WalkDir(pair[0], func(p string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(pair[0], p)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, pair[1], rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o700)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, 0o600)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestM3A3RetainedGitProvenanceAcrossOtherWrites(t *testing.T) {
	for _, kind := range []string{"preserve", "forged-commit", "forged-object"} {
		t.Run(kind, func(t *testing.T) {
			s, home, config, _ := a3Fixture(t, "codex-user")
			ctx := context.Background()
			if _, err := s.Install(ctx, a3Request(home, "codex-user")); err != nil {
				t.Fatal(err)
			}
			original := *mustLoadNativeLock(t, home).Packages[0].Source.Git
			other := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "other"), a6Manifest("other", 50))
			request := a3Request(home, "codex-user")
			request.Source = other
			request.Path = ""
			if _, err := s.Install(ctx, request); err != nil {
				t.Fatal(err)
			}
			if *mustLoadNativeLock(t, home).Packages[0].Source.Git != original {
				t.Fatal("other install discarded Git provenance")
			}
			if kind != "preserve" {
				lock := mustLoadNativeLock(t, home)
				if kind == "forged-commit" {
					lock.Packages[0].Source.Git.Commit = strings.Repeat("c", 40)
				} else {
					lock.Packages[0].Source.Git.Object = strings.Repeat("d", 40)
				}
				if err := writeLock(home, lock); err != nil {
					t.Fatal(err)
				}
			}
			before := a3Installed(t, home, config)
			third := writeInstallPackage(t, filepath.Join(filepath.Dir(home), "third"), a6Manifest("third", 50))
			request.Source = third
			_, installErr := s.Install(ctx, request)
			_, removeErr := s.Uninstall(ctx, UninstallRequest{Home: home, Name: "other", Yes: true})
			if kind == "preserve" {
				if installErr != nil || removeErr != nil {
					t.Fatalf("%v %v", installErr, removeErr)
				}
				lock := mustLoadNativeLock(t, home)
				if lock.Format != 2 || *lock.Packages[0].Source.Git != original {
					t.Fatal("remaining lock lost Git provenance")
				}
			} else {
				if installErr == nil || removeErr == nil || !reflect.DeepEqual(before, a3Installed(t, home, config)) {
					t.Fatalf("forged provenance reapproved: install=%v remove=%v", installErr, removeErr)
				}
			}
		})
	}
}
