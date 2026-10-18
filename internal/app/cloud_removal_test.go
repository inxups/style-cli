package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func a19Project(t *testing.T) (*Service, string, string) {
	t.Helper()
	s, base, root, source := a6Setup(t)
	s = a6Scope(t, s, root, base)
	if _, err := s.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	return s, base, root
}

func TestM3A19ExportRemovedWithoutWrites(t *testing.T) {
	s, base, root, _ := a6Setup(t)
	for _, args := range [][]string{{"export", "--help"}, {"export", "--target=claude-cloud", "--yes"}, {"export", "--target=claude-cloud", "--project", root, "--yes", "--json"}} {
		before := a4State(t, base, root)
		var out, errOut bytes.Buffer
		if code := RunWithService(context.Background(), s, root, args, &out, &errOut); code != ExitUsage || errOut.Len() == 0 {
			t.Fatal(code, &out, &errOut)
		}
		a4AssertState(t, before, base, root)
	}
	var out bytes.Buffer
	printUsage(&out)
	if strings.Contains(out.String(), "export") || strings.Contains(out.String(), "claude-cloud") {
		t.Fatal("removed command is still advertised", &out)
	}
}

func TestM3A19RemoteRejectsBoundAndUnboundProjects(t *testing.T) {
	for _, clone := range []bool{false, true} {
		for _, closed := range []bool{false, true} {
			t.Run(map[bool]string{false: "bound", true: "clone"}[clone]+map[bool]string{false: "-open", true: "-closed"}[closed], func(t *testing.T) {
				_, base, root := a19Project(t)
				if clone {
					copy := filepath.Join(filepath.Dir(root), "unbound clone")
					if err := os.CopyFS(copy, os.DirFS(root)); err != nil {
						t.Fatal(err)
					}
					root = copy
				}
				t.Chdir(root)
				t.Setenv("CLAUDE_CODE_REMOTE", "true")
				t.Setenv("HOME", t.TempDir())
				args := []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}
				if closed {
					args = append(args, "--fail-closed")
				}
				before := a4State(t, filepath.Dir(base), os.Getenv("HOME"))
				var out, errOut bytes.Buffer
				want := 0
				if closed {
					want = 1
				}
				payload := `{"hook_event_name":"PreToolUse","session_id":"removed-cloud","tool_name":"Bash","tool_input":{"command":"true"}}`
				if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &out, &errOut); code != want || !strings.Contains(errOut.String(), "no longer supported") || strings.Contains(out.String(), "Project specific") {
					t.Fatal(code, &out, &errOut)
				}
				a4AssertState(t, before, filepath.Dir(base), os.Getenv("HOME"))
			})
		}
	}
}

func TestM3A19LocalProjectRunsAndUninstalls(t *testing.T) {
	s, base, root := a19Project(t)
	t.Chdir(root)
	for _, remote := range []string{"", "false"} {
		t.Setenv("CLAUDE_CODE_REMOTE", remote)
		var out, errOut bytes.Buffer
		args := []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}
		payload := `{"hook_event_name":"PreToolUse","session_id":"local` + remote + `","tool_name":"Bash","tool_input":{"command":"true"}}`
		if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &out, &errOut); code != 0 || !strings.Contains(out.String(), "Project specific instructions.") || strings.Contains(errOut.String(), "no longer supported") {
			t.Fatal(code, &out, &errOut)
		}
	}
	before := a4State(t, base, root)
	if _, err := s.Uninstall(context.Background(), UninstallRequest{All: true}); err == nil {
		t.Fatal("uninstall without --yes succeeded")
	}
	a4AssertState(t, before, base, root)
	if result, err := s.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil || !result.Changed {
		t.Fatal(result, err)
	}
	for _, name := range []string{projectDeclarationPath, projectLauncherPath} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("project artifact survived uninstall", name, err)
		}
	}
}
