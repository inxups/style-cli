package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
)

func a7Service(t *testing.T, config string, legacy bool) *Service {
	t.Helper()
	installation := adapter.Installation{Provider: "claude-code", ConfigDir: config, Version: "2.1.315"}
	definition := claude.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation}
	if legacy {
		definition.Facts = claude.DefaultFacts().WithStaticLayout(claude.LegacyStaticSlot)
	}
	s, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func a7Manifest(name, text string) string {
	return fmt.Sprintf("spec: style/v0.3\nname: %s\nversion: 0.1.0\ndescription: rules migration fixture\nrules:\n  - {id: static, target: system, text: '%s'}\n", name, text)
}

func a7Install(t *testing.T, service *Service, home, source string, link bool) InstallResult {
	t.Helper()
	result, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Yes: true, Link: link})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func a7AssertRules(t *testing.T, home, config string, names ...string) {
	t.Helper()
	lock, err := loadLock(home)
	if err != nil {
		t.Fatal(err)
	}
	harness := lock.Harnesses["claude-code"]
	if harness.StaticLayout != claude.StaticSlotAppend || len(harness.StaticFiles) != len(names) {
		t.Fatalf("missing rules metadata: %+v", harness)
	}
	want := map[string]bool{}
	for _, name := range names {
		rel := "rules/style-" + name + ".md"
		data := readSyncFile(t, filepath.Join(config, filepath.FromSlash(rel)))
		if harness.StaticFiles[rel] != hashBytes(data) || !bytes.HasPrefix(data, []byte("<!-- style-rules:v1 "+name+" -->\n")) {
			t.Fatalf("invalid owned rules %s", rel)
		}
		want["style-"+name+".md"] = true
	}
	entries, err := os.ReadDir(filepath.Join(config, "rules"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "style-") && !want[entry.Name()] {
			t.Fatalf("stale/noncanonical rules %s", entry.Name())
		}
	}
	for _, entry := range a6Index(t, home).Static["claude-code"] {
		if entry.Slot != claude.StaticSlotAppend {
			t.Fatalf("legacy slot in new index: %+v", entry)
		}
	}
}

func TestA7FreshRulesCustomRootDynamicAndDedupTakeover(t *testing.T) {
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "custom-claude")
	s := a7Service(t, config, false)
	writeSyncFile(t, filepath.Join(config, "CLAUDE.md"), "用户 instructions\r\n")
	writeSyncFile(t, filepath.Join(config, "rules", "user.md"), "User rules")
	first := writeInstallPackage(t, filepath.Join(root, "first"), a7Manifest("first", "Shared"))
	second := writeInstallPackage(t, filepath.Join(root, "second"), a7Manifest("second", "Distinct"))
	dynamic := writeInstallPackage(t, filepath.Join(root, "dynamic"), "spec: style/v0.3\nname: dynamic\nversion: 0.1.0\ndescription: dynamic\nrules:\n  - {id: prompt, target: event, event: prompt.submitted, text: Dynamic instructions.}\n")
	a7Install(t, s, home, first, false)
	a7Install(t, s, home, second, false)
	a7Install(t, s, home, dynamic, false)
	a7AssertRules(t, home, config, "first", "second")
	if len(a6Index(t, home).Harnesses["claude-code"]["prompt.submitted"]) != 1 {
		t.Fatal("dynamic subscription lost")
	}
	a6AssertDoctor(t, s, home)
	alias := writeInstallPackage(t, filepath.Join(root, "alias"), a7Manifest("alias", "Shared"))
	a7Install(t, s, home, alias, false)
	a7AssertRules(t, home, config, "first", "second")
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "first", Yes: true}); err != nil {
		t.Fatal(err)
	}
	a7AssertRules(t, home, config, "alias", "second")
	a6AssertDoctor(t, s, home)
	if repeat, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || repeat.Changed {
		t.Fatalf("repeat sync: %+v %v", repeat, err)
	}
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if string(readSyncFile(t, filepath.Join(config, "CLAUDE.md"))) != "用户 instructions\r\n" || string(readSyncFile(t, filepath.Join(config, "rules", "user.md"))) != "User rules" {
		t.Fatal("foreign instructions changed")
	}
	for _, name := range []string{"first", "second", "alias", "dynamic"} {
		if _, err := os.Stat(filepath.Join(config, "rules", "style-"+name+".md")); !os.IsNotExist(err) {
			t.Fatal("owned file remains after uninstall")
		}
	}
}

func TestA7LegacyMigrationEveryRebuildEntrypoint(t *testing.T) {
	for _, entrypoint := range []string{"install", "sync", "trust", "uninstall", "uninstall-all"} {
		for name, original := range map[string]string{"lf": "User\n", "crlf": "用户\r\n", "no-newline": "用户", "absent": ""} {
			t.Run(entrypoint+"/"+name, func(t *testing.T) {
				root := t.TempDir()
				home, config := filepath.Join(root, "style"), filepath.Join(root, "claude")
				if name != "absent" {
					writeSyncFile(t, filepath.Join(config, "CLAUDE.md"), original)
					if err := os.Chmod(filepath.Join(config, "CLAUDE.md"), 0o640); err != nil {
						t.Fatal(err)
					}
				}
				old := a7Service(t, config, true)
				first := writeInstallPackage(t, filepath.Join(root, "first"), a7Manifest("first", "First"))
				second := writeInstallPackage(t, filepath.Join(root, "second"), a7Manifest("second", "Second"))
				a7Install(t, old, home, first, false)
				a7Install(t, old, home, second, false)
				lock, err := loadLock(home)
				if err != nil || lock.Harnesses["claude-code"].StaticLayout != "" {
					t.Fatal("fixture must have legacy lock")
				}
				s := a7Service(t, config, false)
				before := installedTree(t, home, config)
				doctor, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
				if err != nil || !doctor.Healthy {
					t.Fatalf("valid legacy installation rejected: %+v %v", doctor.Diagnostics, err)
				}
				pending := false
				for _, d := range doctor.Diagnostics {
					if d.Code == "static-migration-pending" {
						pending = true
					}
				}
				listed, err := s.List(context.Background(), ListRequest{Home: home})
				if err != nil || len(listed.Packages) != 2 || !pending {
					t.Fatalf("legacy inventory/pending: %+v %v", listed, err)
				}
				if !reflect.DeepEqual(before, installedTree(t, home, config)) {
					t.Fatal("read-only commands migrated legacy installation")
				}
				var migrations []adapter.StaticMigration
				want := []string{"first", "second"}
				switch entrypoint {
				case "install":
					third := writeInstallPackage(t, filepath.Join(root, "third"), a7Manifest("third", "Third"))
					migrations = a7Install(t, s, home, third, false).StaticMigrations
					want = append(want, "third")
				case "sync":
					r, err := s.Sync(context.Background(), SyncRequest{Home: home})
					if err != nil || len(r.NeedsTrust) != 0 || !r.Changed {
						t.Fatalf("carrier migration requires trust: %+v %v", r, err)
					}
					migrations = r.StaticMigrations
				case "trust":
					r, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Yes: true})
					if err != nil {
						t.Fatal(err)
					}
					migrations = r.StaticMigrations
				case "uninstall", "uninstall-all":
					r, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, Name: map[bool]string{true: "", false: "first"}[entrypoint == "uninstall-all"], All: entrypoint == "uninstall-all", Yes: true})
					if err != nil {
						t.Fatal(err)
					}
					migrations = r.StaticMigrations
					want = []string{"second"}
				}
				if len(migrations) != 2 {
					t.Fatalf("migration report missing: %+v", migrations)
				}
				var report bytes.Buffer
				writeStaticMigrations(&report, migrations)
				if !strings.Contains(report.String(), filepath.Join(config, "CLAUDE.md")) {
					t.Fatal("text report omits old carrier")
				}
				if name == "absent" {
					if _, err := os.Stat(filepath.Join(config, "CLAUDE.md")); !os.IsNotExist(err) {
						t.Fatal("STYLE-only legacy file remains")
					}
				} else {
					if string(readSyncFile(t, filepath.Join(config, "CLAUDE.md"))) != original {
						t.Fatal("migration changed external legacy bytes")
					}
					if info, err := os.Stat(filepath.Join(config, "CLAUDE.md")); err != nil || info.Mode().Perm() != 0o640 {
						t.Fatal("migration changed external legacy mode")
					}
				}
				if entrypoint != "uninstall-all" {
					a7AssertRules(t, home, config, want...)
					a6AssertDoctor(t, s, home)
					if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || r.Changed {
						t.Fatalf("migration not idempotent: %+v %v", r, err)
					}
				}
			})
		}
	}
}

func TestA7MigrationTrustKeepsOtherUnapprovedSourceAndNative(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(fmt.Sprint(link), func(t *testing.T) {
			root := t.TempDir()
			home, config := filepath.Join(root, "style"), filepath.Join(root, "claude")
			old := a7Service(t, config, true)
			first := writeInstallPackage(t, filepath.Join(root, "first"), a7Manifest("first", "First"))
			secondManifest := a7Manifest("second", "Approved second") + "  - {id: native, target: native, claude-code: {event: Stop, command: 'printf approved'}}\n"
			second := writeInstallPackage(t, filepath.Join(root, "second"), secondManifest)
			a7Install(t, old, home, first, false)
			a7Install(t, old, home, second, link)
			if !link {
				second = filepath.Join(home, "packages", "second")
			}
			writeSyncFile(t, filepath.Join(home, "packages", "first", "STYLE.yaml"), a7Manifest("first", "New first"))
			writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), strings.ReplaceAll(strings.ReplaceAll(secondManifest, "Approved second", "Unapproved second"), "printf approved", "printf unapproved"))
			s := a7Service(t, config, false)
			lockBefore, _ := loadLock(home)
			approvalBefore := readSyncFile(t, approvalPath(home, "second"))
			result, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Yes: true})
			if err != nil || !reflect.DeepEqual(result.NeedsTrust, []string{"second"}) {
				t.Fatalf("trust: %+v %v", result, err)
			}
			a7AssertRules(t, home, config, "first", "second")
			body := readSyncFile(t, filepath.Join(config, "rules", "style-second.md"))
			if !bytes.Contains(body, []byte("Approved second")) || bytes.Contains(body, []byte("Unapproved")) {
				t.Fatal("migration approved unrelated body")
			}
			settings := readSyncFile(t, filepath.Join(config, "settings.json"))
			if bytes.Contains(settings, []byte("printf unapproved")) || !bytes.Contains(settings, []byte("printf approved")) {
				t.Fatal("migration approved unrelated native command")
			}
			lockAfter, _ := loadLock(home)
			before, _ := findLockedPackage(lockBefore, "second")
			after, _ := findLockedPackage(lockAfter, "second")
			// Only the rendered carrier changes in the approval, never its source hashes.
			if !reflect.DeepEqual(before.Files, after.Files) {
				t.Fatal("unapproved source hashes accepted")
			}
			baseline, _, err := loadApproval(home, after)
			if err != nil || bytes.Contains(baseline.Files["STYLE.yaml"], []byte("Unapproved")) {
				t.Fatal("unapproved bytes entered baseline")
			}
			if len(approvalBefore) == 0 {
				t.Fatal("missing old approval")
			}
		})
	}
}

func a7Tree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				result[path] = "symlink:" + target
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[path] = fmt.Sprintf("%o:%s", info.Mode().Perm(), data)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestA7MigrationConflictsLeaveFullTransactionUnchanged(t *testing.T) {
	for _, kind := range []string{"foreign", "same-bytes", "forged-header", "legacy-symlink", "rules-symlink", "file-symlink", "malformed", "unwritable-parent"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			home, config := filepath.Join(root, "style"), filepath.Join(root, "claude")
			source := writeInstallPackage(t, filepath.Join(root, "source"), a7Manifest("first", "Approved"))
			a7Install(t, a7Service(t, config, true), home, source, false)
			path := filepath.Join(config, "rules", "style-first.md")
			outside := filepath.Join(root, "outside")
			writeSyncFile(t, outside, "Outside")
			switch kind {
			case "foreign":
				writeSyncFile(t, path, "User file")
			case "same-bytes":
				writeSyncFile(t, path, "<!-- style-rules:v1 first -->\nApproved\n<!-- /style-rules:v1 first -->\n")
			case "forged-header":
				writeSyncFile(t, path, "<!-- style-rules:v1 first -->\nForged\n<!-- /style-rules:v1 first -->\n")
			case "legacy-symlink":
				if err := os.Remove(filepath.Join(config, "CLAUDE.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(config, "CLAUDE.md")); err != nil {
					t.Fatal(err)
				}
			case "rules-symlink":
				if err := os.Symlink(root, filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			case "file-symlink":
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				writeSyncFile(t, filepath.Join(config, "CLAUDE.md"), "<!-- style:first -->\nUnterminated")
			case "unwritable-parent":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permission checks")
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(filepath.Dir(path), 0o700)
			}
			s := a7Service(t, config, false)
			before := a7Tree(t, home, config, outside)
			if _, err := s.Sync(context.Background(), SyncRequest{Home: home}); err == nil {
				t.Fatal("migration accepted conflict")
			}
			if after := a7Tree(t, home, config, outside); !reflect.DeepEqual(before, after) {
				t.Fatal("failed migration changed config, package, index, lock or approval")
			}
		})
	}
}

func TestA7ManagedRulesDriftAndStaleMigrationPreserveTransaction(t *testing.T) {
	for _, kind := range []string{"drift", "missing", "stale-legacy", "stale-rules"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			home, config := filepath.Join(root, "style"), filepath.Join(root, "claude")
			source := writeInstallPackage(t, filepath.Join(root, "source"), a7Manifest("first", "Approved"))
			a7Install(t, a7Service(t, config, kind == "stale-legacy"), home, source, false)
			s := a7Service(t, config, false)
			path := filepath.Join(config, "rules", "style-first.md")
			if kind == "drift" {
				writeSyncFile(t, path, "Changed")
			}
			if kind == "missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(kind, "stale-") {
				writeSyncFile(t, filepath.Join(home, "packages", "first", "STYLE.yaml"), a7Manifest("first", "Updated"))
				if kind == "stale-legacy" {
					path = filepath.Join(config, "CLAUDE.md")
				}
			}
			before := a7Tree(t, home, config)
			if strings.HasPrefix(kind, "stale-") {
				_, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "first", Confirm: func(SyncResult) (bool, error) { writeSyncFile(t, path, "External edit"); return true, nil }})
				if err == nil {
					t.Fatal("stale migration accepted")
				}
				before[path] = a7Tree(t, path)[path]
			} else {
				if _, err := s.Sync(context.Background(), SyncRequest{Home: home}); err == nil {
					t.Fatal("owned rules drift accepted")
				}
				if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err == nil {
					t.Fatal("owned rules drift deleted")
				}
				d, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
				if err != nil || d.Healthy {
					t.Fatal("doctor missed rules drift")
				}
			}
			if !reflect.DeepEqual(before, a7Tree(t, home, config)) {
				t.Fatal("failed transaction changed protected files")
			}
		})
	}
}
