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

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

func a6Setup(t *testing.T) (*Service, string, string, string) {
	requirePOSIXFixture(t, "project/plugin delivery requires POSIX")
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, "private")
	project := filepath.Join(root, "project ' space")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	c := adapter.Installation{Provider: codex.ProviderName, ConfigDir: filepath.Join(root, "codex-user"), Version: "0.159.3"}
	a := adapter.Installation{Provider: claude.ProviderName, ConfigDir: filepath.Join(root, "claude-user"), Version: "2.1.301"}
	cd := codex.DefinitionForInstallation(c)
	cd.Detector = fixedDetector{c}
	ad := claude.DefinitionForInstallation(a)
	ad.Detector = fixedDetector{a}
	s, err := New(cd, ad)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STYLE_HOME", base)
	t.Setenv("CODEX_HOME", c.ConfigDir)
	t.Setenv("CLAUDE_CONFIG_DIR", a.ConfigDir)
	source := writeInstallPackage(t, filepath.Join(root, "source"), a4Manifest("project-rules", "Project specific instructions."))
	return s, base, project, source
}
func a6Scope(t *testing.T, s *Service, project, base string) *Service {
	t.Helper()
	value, err := s.ForProject(project, base)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func a6Install(t *testing.T, s *Service, source string) {
	t.Helper()
	if _, err := s.Install(context.Background(), InstallRequest{Source: source, Yes: true}); err != nil {
		t.Fatal(err)
	}
}

func TestM3A6ProjectLifecycle(t *testing.T) {
	for _, id := range []string{"codex", "claude-code"} {
		t.Run(id, func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			scoped := a6Scope(t, s, root, base)
			first, err := scoped.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{id}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if !first.Changed || len(first.Routes) != 1 || !strings.Contains(first.Routes[0], "-project") {
				t.Fatalf("%+v", first)
			}
			lock, err := scoped.loadLock(scoped.project.Home)
			if err != nil || lock.Format != 3 || lock.Project == nil {
				t.Fatalf("%+v %v", lock, err)
			}
			for _, name := range []string{"style.lock.yaml", "binding.json", "build/index.json", "build/explain.json", "approved/project-rules.json"} {
				if _, err = os.Stat(filepath.Join(scoped.project.Home, name)); err != nil {
					t.Fatal(name, err)
				}
			}
			d, err := readProjectDeclaration(scoped.project.Root)
			if err != nil || d == nil {
				t.Fatal(d, err)
			}
			raw := readSyncFile(t, filepath.Join(root, projectDeclarationPath))
			if bytes.Contains(raw, []byte(base)) || bytes.Contains(raw, []byte(root)) {
				t.Fatal("private path in portable declaration")
			}
			second, err := scoped.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{id}, Yes: true})
			if err != nil || second.Changed {
				t.Fatalf("idempotent=%+v err=%v", second, err)
			}
			list, err := scoped.List(context.Background(), ListRequest{})
			if err != nil || len(list.Packages) != 1 {
				t.Fatal(list, err)
			}
			sync, err := scoped.Sync(context.Background(), SyncRequest{})
			if err != nil || sync.Changed {
				t.Fatal(sync, err)
			}
			update, err := scoped.Update(context.Background(), UpdateRequest{All: true, Yes: true})
			if err != nil || update.Changed {
				t.Fatal(update, err)
			}
			check := a5Lock(t, scoped.project.Home)
			a5Offline(t, scoped)
			frozen, err := scoped.InstallFrozen(context.Background(), FrozenRequest{Yes: true})
			if err != nil || frozen.Changed {
				t.Fatal(frozen, err)
			}
			check()
			removed, err := scoped.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true})
			if err != nil || !removed.Changed {
				t.Fatal(removed, err)
			}
			for _, name := range []string{projectDeclarationPath, projectLauncherPath} {
				if _, e := os.Stat(filepath.Join(root, name)); !errors.Is(e, os.ErrNotExist) {
					t.Fatalf("remains %s %v", name, e)
				}
			}
		})
	}
}
func TestM3A6ClaudeProjectStaticOnlyNeedsNoSettings(t *testing.T) {
	s, base, root, hooked := a6Setup(t)
	static := writeInstallPackage(t, filepath.Join(filepath.Dir(root), "static"),
		"spec: style/v0.3\nname: static-only\nversion: 1.0.0\ndescription: x\nrules:\n  - {id: static, target: system, text: hi}\n")
	claudeOnly := []string{"claude-code"}
	scoped := a6Scope(t, s, root, base)
	if _, err := scoped.Install(context.Background(), InstallRequest{Source: static, Harnesses: claudeOnly, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude/rules/style-static-only.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude/settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("static-only install wrote settings.json: %v", err)
	}
	clone := filepath.Join(filepath.Dir(root), "clone")
	if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	if _, err := a6Scope(t, s, clone, base).Install(context.Background(), InstallRequest{Source: static, Harnesses: claudeOnly, Yes: true}); err != nil {
		t.Fatal("static-only clone:", err)
	}
	if _, err := scoped.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{projectDeclarationPath, projectLauncherPath, ".claude/rules/style-static-only.md", ".claude/settings.json"} {
		if _, e := os.Stat(filepath.Join(root, name)); !errors.Is(e, os.ErrNotExist) {
			t.Fatalf("remains %s %v", name, e)
		}
	}

	// A clone still must carry the hook settings its declaration names.
	if _, err := scoped.Install(context.Background(), InstallRequest{Source: hooked, Harnesses: claudeOnly, Yes: true}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(filepath.Dir(root), "missing")
	if err := os.CopyFS(missing, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(missing, ".claude/settings.json")); err != nil {
		t.Fatal(err)
	}
	_, err := a6Scope(t, s, missing, base).Install(context.Background(), InstallRequest{Source: hooked, Harnesses: claudeOnly, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "missing a declared hook artifact") {
		t.Fatalf("clone without declared settings.json: %v", err)
	}
}
func TestM3A6ScopesAreIndependent(t *testing.T) {
	s, base, root, source := a6Setup(t)
	other := filepath.Join(filepath.Dir(root), "second")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	a := a6Scope(t, s, root, base)
	b := a6Scope(t, s, other, base)
	a6Install(t, a, source)
	a6Install(t, b, source)
	if a.project.Home == b.project.Home {
		t.Fatal("shared storage")
	}
	before := a4State(t, a.project.Home, root)
	if _, err := b.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a4AssertState(t, before, a.project.Home, root)
	if _, err := a.Sync(context.Background(), SyncRequest{}); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(root), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	c := a6Scope(t, s, alias, base)
	if c.project.Home != a.project.Home {
		t.Fatal("alias got separate storage")
	}
}
func TestM3A6CrossScopeSameNameRejectedBothDirections(t *testing.T) {
	for _, first := range []string{"user", "project"} {
		t.Run(first, func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			project := a6Scope(t, s, root, base)
			a, b := s, project
			if first == "project" {
				a, b = project, s
			}
			a6Install(t, a, source)
			before := a4State(t, base, root)
			_, err := b.Install(context.Background(), InstallRequest{Source: source, Yes: true})
			if err == nil || !strings.Contains(err.Error(), "another scope") {
				t.Fatal(err)
			}
			a4AssertState(t, before, base, root)
		})
	}
}
func TestM3A6ProjectCLIAndZeroWriteFailures(t *testing.T) {
	for _, args := range [][]string{{"list", "--project"}, {"list", "--project="}, {"list", "--project", "--json"}, {"list", "--project=missing", "--json"}, {"init", "--project=."}, {"validate", "--project=."}, {"list", "--project=.", "--project=."}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			s, base, root, _ := a6Setup(t)
			before := a4State(t, base, root)
			var out, errOut bytes.Buffer
			if code := RunWithService(context.Background(), s, root, args, &out, &errOut); code != 1 || errOut.Len() == 0 {
				t.Fatal(code, &out, &errOut)
			}
			a4AssertState(t, before, base, root)
		})
	}
	s, base, root, source := a6Setup(t)
	var out, errOut bytes.Buffer
	if code := RunWithServiceInput(context.Background(), s, root, []string{"install", source, "--project", root, "--json"}, approve(), &out, &errOut); code != 0 {
		t.Fatal(code, &out, &errOut)
	}
	var report map[string]json.RawMessage
	if json.Unmarshal(out.Bytes(), &report) != nil || report["scope"] == nil {
		t.Fatal(&out)
	}
	for _, command := range []string{"list", "doctor", "stats"} {
		out.Reset()
		errOut.Reset()
		if code := RunWithService(context.Background(), s, root, []string{command, "--project=.", "--json"}, &out, &errOut); code != 0 {
			t.Fatal(command, code, &out, &errOut)
		}
	}
	user, err := s.List(context.Background(), ListRequest{Home: base})
	if err != nil || len(user.Packages) != 0 {
		t.Fatal(user, err)
	}
}
func TestM3A6NestedAndProtectedRoots(t *testing.T) {
	s, base, root, source := a6Setup(t)
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	nested := filepath.Join(root, "child")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{nested, base, filepath.Join(base, "child"), os.Getenv("CODEX_HOME"), os.Getenv("CLAUDE_CONFIG_DIR")} {
		if err := os.MkdirAll(candidate, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ForProject(candidate, base); err == nil {
			t.Fatal("accepted", candidate)
		}
	}
	if _, err := s.ForProject(filepath.Join(root, "absent"), base); err == nil {
		t.Fatal("accepted absent directory")
	}
}
func TestM3A6VersionPrerequisitesAndWrongRoutes(t *testing.T) {
	for _, id := range []adapter.ProviderID{codex.ProviderName, claude.ProviderName} {
		for _, version := range []string{"", "unknown", "0.1.0"} {
			t.Run(string(id)+version, func(t *testing.T) {
				s, base, root, source := a6Setup(t)
				d := s.definitions[id]
				inst, _, _ := d.Detect(context.Background())
				inst.Version = version
				d.Detector = fixedDetector{inst}
				s.definitions[id] = d
				scoped := a6Scope(t, s, root, base)
				before := a4State(t, base, root)
				if _, err := scoped.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{string(id)}, Yes: true}); err == nil {
					t.Fatal("accepted unsupported version")
				}
				a4AssertState(t, before, base, root)
			})
		}
	}
	s, base, root, source := a6Setup(t)
	scoped := a6Scope(t, s, root, base)
	for _, route := range []string{"codex-user", "codex-plugin"} {
		if _, err := scoped.Install(context.Background(), InstallRequest{Source: source, Routes: map[string]string{"codex": route}, Yes: true}); err == nil {
			t.Fatal(route)
		}
	}
}
func TestM3A6CloneNeedsFreshBindingAndMatchingEffects(t *testing.T) {
	s, base, root, source := a6Setup(t)
	a := a6Scope(t, s, root, base)
	a6Install(t, a, source)
	clone := filepath.Join(filepath.Dir(root), "clone")
	if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	b := a6Scope(t, s, clone, base)
	if _, err := b.InstallFrozen(context.Background(), FrozenRequest{Yes: true}); err == nil {
		t.Fatal("clone inherited frozen trust")
	}
	before := a4State(t, base, clone)
	preview, err := b.Install(context.Background(), InstallRequest{Source: source, Confirm: func(preview InstallResult) (bool, error) {
		if !preview.ProjectAdoption {
			t.Fatal("missing clone ownership disclosure")
		}
		return false, nil
	}})
	if err == nil || !preview.ProjectAdoption || !preview.AwaitingConfirmation {
		t.Fatal(preview, err)
	}
	a4AssertState(t, before, base, clone)
	a6Install(t, b, source)
	la, _ := a.loadLock(a.project.Home)
	lb, _ := b.loadLock(b.project.Home)
	if reflect.DeepEqual(la.Project, lb.Project) {
		t.Fatal("clone inherited binding")
	}
	if _, err := b.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".codex/hooks.json", ".claude/settings.json", ".claude/rules/style-project-rules.md"} {
		if data, e := os.ReadFile(filepath.Join(clone, name)); e == nil && bytes.Contains(data, []byte("--owner=style")) {
			t.Fatal("adopted hook survived uninstall", name)
		}
	}
}
