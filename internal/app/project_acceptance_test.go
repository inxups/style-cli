package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"gopkg.in/yaml.v3"
)

func a6Batch(t *testing.T) (*Service, string, string, []string) {
	t.Helper()
	s, base, root, first := a6Setup(t)
	writeSyncFile(t, filepath.Join(first, "STYLE.yaml"), strings.Replace(nativePackageYAML("project-rules"), "rules:", "rules:\n  - {id: static, target: system, text: 'Project first static.'}", 1))
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(root), "second-source"), strings.Replace(a4Manifest("second", "Use {{settings.voice}}."), "rules:", "settings: [{name: voice, default: DefaultVoice}]\nrules:", 1))
	scoped := a6Scope(t, s, root, base)
	result, err := scoped.Install(context.Background(), InstallRequest{Source: first, ProjectSources: []string{second}, ProjectSettings: map[string]string{"second.voice": "SelectedVoice"}, Yes: true})
	if err != nil || len(result.ProjectPackages) != 1 {
		t.Fatal(result, err)
	}
	return scoped, base, root, []string{first, second}
}
func TestM3A6MultiPackageCloneAdoptionAndPartialUninstall(t *testing.T) {
	s, base, root, sources := a6Batch(t)
	clone := filepath.Join(filepath.Dir(root), "multi-clone")
	if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	b := a6Scope(t, s, clone, base)
	before := a4State(t, base, clone)
	for _, request := range []InstallRequest{
		{Source: sources[0], Yes: true},
		{Source: sources[0], ProjectSources: []string{sources[0]}, Yes: true},
		{Source: sources[0], ProjectSources: []string{sources[1]}, Yes: true},
		{Source: sources[0], ProjectSources: []string{sources[1]}, ProjectSettings: map[string]string{"absent.voice": "bad"}, Yes: true},
	} {
		if _, err := b.Install(context.Background(), request); err == nil {
			t.Fatal("adopted incomplete/different candidate")
		}
		a4AssertState(t, before, base, clone)
	}
	request := InstallRequest{Source: sources[0], ProjectSources: []string{sources[1]}, ProjectSettings: map[string]string{"second.voice": "SelectedVoice"}, Yes: true}
	if _, err := b.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	la, _ := s.loadLock(s.project.Home)
	lb, err := b.loadLock(b.project.Home)
	if err != nil || len(lb.Packages) != 2 || la.Project.Root == lb.Project.Root {
		t.Fatal(lb, err)
	}
	if _, err = b.Uninstall(context.Background(), UninstallRequest{Name: "project-rules", Yes: true}); err != nil {
		t.Fatal(err)
	}
	lb, err = b.loadLock(b.project.Home)
	if err != nil || len(lb.Packages) != 1 || lb.Packages[0].Name != "second" || lb.Packages[0].Seq != 2 {
		t.Fatal(lb, err)
	}
	if _, err = b.Sync(context.Background(), SyncRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.InstallFrozen(context.Background(), FrozenRequest{Yes: true}); err != nil {
		t.Fatal(err)
	}
	// A declaration preserves sequence gaps when an installed project is cloned.
	clone2 := filepath.Join(filepath.Dir(root), "partial-clone")
	if err = os.CopyFS(clone2, os.DirFS(clone)); err != nil {
		t.Fatal(err)
	}
	c := a6Scope(t, s, clone2, base)
	if _, err = c.Install(context.Background(), InstallRequest{Source: sources[1], Settings: map[string]string{"voice": "SelectedVoice"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, scoped := range []*Service{b, c} {
		if _, err = scoped.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Sync(context.Background(), SyncRequest{}); err != nil {
		t.Fatal("another clone was changed", err)
	}
}
func TestM3A6CloneRejectsChangedDynamicAndNativeEffects(t *testing.T) {
	for _, change := range []string{"dynamic", "native", "missing-hook", "foreign-style", "another-home"} {
		t.Run(change, func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			manifest := nativePackageYAML("project-rules")
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
			scoped := a6Scope(t, s, root, base)
			a6Install(t, scoped, source)
			clone := filepath.Join(filepath.Dir(root), "clone")
			if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "dynamic":
				writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(manifest, "Be careful", "Different dynamic"))
			case "native":
				writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(manifest, "printf earlier", "printf different"))
			case "missing-hook":
				if err := os.Remove(filepath.Join(clone, ".codex/hooks.json")); err != nil {
					t.Fatal(err)
				}
			case "foreign-style":
				if err := os.Remove(filepath.Join(clone, projectDeclarationPath)); err != nil {
					t.Fatal(err)
				}
			case "another-home":
				base = filepath.Join(filepath.Dir(base), "other-private")
			}
			b := a6Scope(t, s, clone, base)
			before := a4State(t, base, clone)
			if change == "another-home" {
				a6Install(t, b, source)
				return
			} // Independent explicit reapproval is permitted.
			if _, err := b.Install(context.Background(), InstallRequest{Source: source, Yes: true}); err == nil {
				t.Fatal("adopted altered clone")
			}
			a4AssertState(t, before, base, clone)
		})
	}
}
func TestM3A6StaticPlacementAndForeignFilesPreserved(t *testing.T) {
	s, base, root, source := a6Setup(t)
	userConfig := filepath.Join(os.Getenv("CODEX_HOME"), "config.toml")
	userClaude := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "rules", "user.md")
	foreign := map[string]string{userConfig: "# original user\ndeveloper_instructions = 'Keep user instructions.'\n", userClaude: "Keep user Claude rule.", filepath.Join(root, "CLAUDE.md"): "Foreign project instruction.", filepath.Join(root, ".claude/settings.json"): "{\"permissions\":{\"allow\":[\"Read\"]}}\n", filepath.Join(root, ".codex/hooks.json"): "{\"unrelated\":true}\n"}
	for path, body := range foreign {
		writeSyncFile(t, path, body)
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	var index compile.Index
	if err := json.Unmarshal(readSyncFile(t, filepath.Join(scoped.project.Home, "build/index.json")), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Static["codex"]) != 0 || len(index.Static["claude-code"]) != 1 {
		t.Fatal(index.Static)
	}
	var degraded bool
	for _, entries := range index.Harnesses["codex"] {
		for _, entry := range entries {
			degraded = degraded || entry.DegradedFrom != nil
		}
	}
	if !degraded {
		t.Fatal("Codex static append did not use runtime fallback")
	}
	if !bytes.Contains(readSyncFile(t, userConfig), []byte("Keep user instructions")) {
		t.Fatal("overrode user static")
	}
	if _, err := os.Stat(filepath.Join(root, ".codex/config.toml")); !os.IsNotExist(err) {
		t.Fatal("project trust file", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude/rules/style-project-rules.md")); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"codex-project", "claude-project"} {
		if !strings.Contains(string(readSyncFile(t, filepath.Join(scoped.project.Home, "style.lock.yaml"))), route) {
			t.Fatal(route)
		}
	}
	if _, err := scoped.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for path, body := range foreign {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o640 || string(readSyncFile(t, path)) != body {
			t.Fatal("foreign bytes/mode not restored", path, err)
		}
	}
}
func TestM3A6RegistryAndBindingCorruptionRejectManagement(t *testing.T) {
	for _, damage := range []string{"registry-missing", "registry-trailing", "registry-stale", "lock-root", "lock-format", "lock-harness", "binding-symlink", "declaration-trailing", "declaration-static"} {
		t.Run(damage, func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			scoped := a6Scope(t, s, root, base)
			a6Install(t, scoped, source)
			registry := filepath.Join(base, "project-registry.json")
			lockPath := filepath.Join(scoped.project.Home, "style.lock.yaml")
			switch damage {
			case "registry-missing":
				if err := os.Remove(registry); err != nil {
					t.Fatal(err)
				}
			case "registry-trailing":
				writeSyncFile(t, registry, string(readSyncFile(t, registry))+" {}")
			case "registry-stale":
				writeSyncFile(t, registry, strings.ReplaceAll(string(readSyncFile(t, registry)), "project-rules", "different"))
			case "lock-root", "lock-format", "lock-harness":
				lock, err := loadLock(scoped.project.Home)
				if err != nil {
					t.Fatal(err)
				}
				switch damage {
				case "lock-root":
					lock.Project.Root += "-moved"
				case "lock-format":
					lock.Format = 2
				case "lock-harness":
					h := lock.Harnesses["codex"]
					h.Route = codex.UserRouteName
					lock.Harnesses["codex"] = h
				}
				data, _ := yaml.Marshal(lock)
				writeSyncFile(t, lockPath, string(data))
			case "binding-symlink":
				path := filepath.Join(scoped.project.Home, "binding.json")
				if err := os.Rename(path, path+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".saved", path); err != nil {
					t.Fatal(err)
				}
			case "declaration-trailing":
				path := filepath.Join(root, projectDeclarationPath)
				writeSyncFile(t, path, string(readSyncFile(t, path))+" {}")
			case "declaration-static":
				path := filepath.Join(root, projectDeclarationPath)
				writeSyncFile(t, path, strings.ReplaceAll(string(readSyncFile(t, path)), `"claude-code":`, `"unregistered":`))
			}
			before := a4State(t, base, root)
			if _, err := scoped.Sync(context.Background(), SyncRequest{}); err == nil {
				t.Fatal("accepted", damage)
			}
			a4AssertState(t, before, base, root)
		})
	}
}
func TestM3A6LockRejectsWrongScopeAndOldFormats(t *testing.T) {
	s, base, root, source := a6Setup(t)
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	if _, err := s.List(context.Background(), ListRequest{Home: scoped.project.Home}); err == nil {
		t.Fatal("project lock accepted as user")
	}
	lock, _ := loadLock(scoped.project.Home)
	for _, format := range []int{1, 2, 4} {
		bad := lock
		bad.Format = format
		data, _ := yaml.Marshal(bad)
		if _, err := decodeLock(data); err == nil {
			t.Fatal(format)
		}
	}
	bad := lock
	bad.Project = nil
	data, _ := yaml.Marshal(bad)
	if _, err := decodeLock(data); err == nil {
		t.Fatal("format3 without binding")
	}
}
func TestM3A6ProjectBatchValidationAndJSONDiagnostics(t *testing.T) {
	for _, mode := range []string{"user", "linked", "duplicate", "invalid-extra", "unknown-setting"} {
		t.Run(mode, func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			request := InstallRequest{Source: source, ProjectSources: []string{source}, Yes: true}
			switch mode {
			case "user":
			case "linked":
				s = a6Scope(t, s, root, base)
				request.Link = true
			case "duplicate":
				s = a6Scope(t, s, root, base)
			case "invalid-extra":
				s = a6Scope(t, s, root, base)
				bad := writeInstallPackage(t, filepath.Join(filepath.Dir(root), "bad"), "spec: style/v0.3\nname: broken\nversion: 1.0.0\nrules: [{id: invalid, target: missing}]\n")
				request.ProjectSources = []string{bad}
			case "unknown-setting":
				s = a6Scope(t, s, root, base)
				request.ProjectSources = nil
				request.ProjectSettings = map[string]string{"absent.key": "x"}
			}
			before := a4State(t, base, root)
			if _, err := s.Install(context.Background(), request); err == nil {
				t.Fatal("accepted", mode)
			}
			a4AssertState(t, before, base, root)
		})
	}
	s, base, root, source := a6Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), "spec: style/v0.3\nname: broken\nversion: 1.0.0\nrules: [{id: invalid, target: missing}]\n")
	before := a4State(t, base, root)
	var out, errOut bytes.Buffer
	if code := RunWithService(context.Background(), s, root, []string{"install", source, "--project=.", "--yes", "--json"}, &out, &errOut); code != 1 {
		t.Fatal(code, &out, &errOut)
	}
	var report map[string]any
	if json.Unmarshal(out.Bytes(), &report) != nil || report["scope"] == nil || report["diagnostics"] == nil {
		t.Fatal(&out, &errOut)
	}
	a4AssertState(t, before, base, root)
}
func TestM3A6ChangedNativeRequiresExplicitTrust(t *testing.T) {
	s, base, root, source := a6Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), nativePackageYAML("project-rules"))
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	path := filepath.Join(scoped.project.Home, "packages/project-rules/STYLE.yaml")
	writeSyncFile(t, path, strings.ReplaceAll(string(readSyncFile(t, path)), "printf earlier", "printf changed"))
	before := a4State(t, base, root)
	result, err := scoped.Sync(context.Background(), SyncRequest{})
	if err != nil || !reflect.DeepEqual(result.NeedsTrust, []string{"project-rules"}) || result.Changed {
		t.Fatal(result, err)
	}
	a4AssertState(t, before, base, root)
	approved, err := scoped.Trust(context.Background(), TrustRequest{Name: "project-rules", Yes: true})
	if err != nil || !approved.Changed {
		t.Fatal(approved, err)
	}
	for _, name := range []string{".codex/hooks.json", ".claude/settings.json"} {
		if !bytes.Contains(readSyncFile(t, filepath.Join(root, name)), []byte("printf changed")) {
			t.Fatal(name)
		}
	}
	if _, err = scoped.Sync(context.Background(), SyncRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = scoped.InstallFrozen(context.Background(), FrozenRequest{Yes: true}); err != nil {
		t.Fatal(err)
	}
}
func TestM3A6ProjectReplaceDoesNotGainUnsupportedStaticSlot(t *testing.T) {
	s, base, root, source := a6Setup(t)
	manifest := strings.ReplaceAll(a4Manifest("project-rules", "Replace instructions."), "target: system", "target: system, mode: replace")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
	scoped := a6Scope(t, s, root, base)
	before := a4State(t, base, root)
	_, err := scoped.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{string(claude.ProviderName)}, Yes: true})
	if err == nil {
		t.Fatal("replace confirmation was bypassed")
	}
	a4AssertState(t, before, base, root)
	_, err = scoped.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{string(claude.ProviderName)}, AllowReplace: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	var index compile.Index
	if err = json.Unmarshal(readSyncFile(t, filepath.Join(scoped.project.Home, "build/index.json")), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Static["claude-code"]) != 0 {
		t.Fatal("unsupported replacement gained a static slot")
	}
	for _, entries := range index.Harnesses["claude-code"] {
		for _, entry := range entries {
			if entry.Rule == "project-rules/static" {
				t.Fatal("unsupported replacement was injected", entry)
			}
		}
	}
}

func TestM3A6DifferentNamesCoexistAndForeignUserHomeIsRejected(t *testing.T) {
	s, base, root, source := a6Setup(t)
	userSource := writeInstallPackage(t, filepath.Join(filepath.Dir(root), "user-source"), a4Manifest("user-rules", "Keep user scope."))
	a6Install(t, s, userSource)
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	userBefore := a4State(t, filepath.Join(base, "packages"), os.Getenv("CLAUDE_CONFIG_DIR"))
	list, err := s.List(context.Background(), ListRequest{})
	if err != nil || len(list.Packages) != 1 || list.Packages[0].Name != "user-rules" {
		t.Fatal(list, err)
	}
	if _, err = scoped.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a4AssertState(t, userBefore, filepath.Join(base, "packages"), os.Getenv("CLAUDE_CONFIG_DIR"))
	otherBase := filepath.Join(filepath.Dir(base), "other-base")
	other := a6Scope(t, s, root, otherBase)
	before := a4State(t, filepath.Dir(base), root)
	if _, err = other.Install(context.Background(), InstallRequest{Source: source, Yes: true}); err == nil || !strings.Contains(err.Error(), "outside this base home") {
		t.Fatal(err)
	}
	a4AssertState(t, before, filepath.Dir(base), root)
}
