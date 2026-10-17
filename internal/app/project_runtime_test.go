package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM3A6RuntimeSelectsOnlyProcessProject(t *testing.T) {
	for _, provider := range []string{"codex", "claude-code"} {
		t.Run(provider, func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), nativePackageYAML("project-rules"))
			scoped := a6Scope(t, s, root, base)
			a6Install(t, scoped, source)
			sub := filepath.Join(root, "nested")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			sub, _ = canonicalProjectPath(sub)
			t.Chdir(sub)
			args := []string{"--project-hook", "agenthooks", "run", "--provider=" + provider, "--owner=style"}
			payload := `{"hook_event_name":"PreToolUse","session_id":"same","cwd":"/forged-root","tool_name":"Bash","tool_input":{"command":"true"}}`
			var out, errOut bytes.Buffer
			if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &out, &errOut); code != 0 || !strings.Contains(out.String(), "Be careful") {
				t.Fatal(code, &out, &errOut)
			}
			if _, err := os.Stat(filepath.Join(scoped.project.Home, "state")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(base, "state")); !os.IsNotExist(err) {
				t.Fatal("wrote user state", err)
			}
			// An unbound clone must not fall back to the user's index or create state.
			clone := filepath.Join(filepath.Dir(root), "unbound")
			if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
				t.Fatal(err)
			}
			clone, _ = canonicalProjectPath(clone)
			t.Chdir(clone)
			before := a4State(t, base, clone)
			out.Reset()
			errOut.Reset()
			if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &out, &errOut); code != 0 || strings.Contains(out.String(), "Be careful") || !strings.Contains(errOut.String(), "no local binding") {
				t.Fatal(code, &out, &errOut)
			}
			a4AssertState(t, before, base, clone)
		})
	}
}
func TestM3A6RuntimeBindingFailuresUseNativePolicy(t *testing.T) {
	for _, damage := range []string{"declaration", "binding", "wrong-home", "wrong-harness", "wrong-runner", "nested", "entry-root"} {
		for _, closed := range []bool{false, true} {
			t.Run(damage+map[bool]string{true: "-closed", false: "-open"}[closed], func(t *testing.T) {
				s, base, root, source := a6Setup(t)
				scoped := a6Scope(t, s, root, base)
				a6Install(t, scoped, source)
				t.Chdir(root)
				args := []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}
				if closed {
					args = append(args, "--fail-closed")
				}
				switch damage {
				case "declaration":
					p := filepath.Join(root, projectDeclarationPath)
					data := readSyncFile(t, p)
					writeSyncFile(t, p, string(data)+" ")
				case "binding":
					if err := os.Remove(filepath.Join(scoped.project.Home, "binding.json")); err != nil {
						t.Fatal(err)
					}
				case "wrong-home":
					t.Setenv("STYLE_HOME", filepath.Join(filepath.Dir(base), "different"))
				case "wrong-harness":
					t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(filepath.Dir(base), "different-harness"))
				case "wrong-runner":
					p := filepath.Join(scoped.project.Home, "binding.json")
					data := readSyncFile(t, p)
					var b projectBinding
					if err := json.Unmarshal(data, &b); err != nil {
						t.Fatal(err)
					}
					b.Runner = "/different/style"
					data, _ = json.Marshal(b)
					writeSyncFile(t, p, string(data))
				case "nested":
					child := filepath.Join(root, "child")
					if err := os.Mkdir(child, 0o755); err != nil {
						t.Fatal(err)
					}
					writeSyncFile(t, filepath.Join(child, projectDeclarationPath), string(readSyncFile(t, filepath.Join(root, projectDeclarationPath))))
					t.Chdir(child)
				case "entry-root":
					args = append([]string{"--project-entry-root=/wrong-root"}, args...)
				}
				before := a4State(t, base, root)
				var out, errOut bytes.Buffer
				code := RunWithInput(context.Background(), args, strings.NewReader(`{"hook_event_name":"PreToolUse","session_id":"same","tool_name":"Bash","tool_input":{"command":"true"}}`), &out, &errOut)
				want := 0
				if closed {
					want = 1
				}
				if code != want || errOut.Len() == 0 {
					t.Fatal(code, &out, &errOut)
				}
				a4AssertState(t, before, base, root)
			})
		}
	}
}

func TestM3A6SameSessionAcrossProjectsDoesNotShareRepeatOrStats(t *testing.T) {
	s, base, root, source := a6Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(nativePackageYAML("project-rules"), "target: tool", "repeat: session\n    target: tool"))
	a := a6Scope(t, s, root, base)
	a6Install(t, a, source)
	other := filepath.Join(filepath.Dir(root), "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	b := a6Scope(t, s, other, base)
	a6Install(t, b, source)
	payload := `{"hook_event_name":"PreToolUse","session_id":"same-session","tool_name":"Bash","tool_input":{"command":"true"}}`
	for _, scoped := range []*Service{a, b} {
		t.Chdir(scoped.project.Root)
		var out, errOut bytes.Buffer
		if code := RunWithInput(context.Background(), []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}, strings.NewReader(payload), &out, &errOut); code != 0 || !strings.Contains(out.String(), "Be careful") {
			t.Fatal(code, &out, &errOut)
		}
		if _, err := os.Stat(filepath.Join(scoped.project.Home, "state")); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"state", "stats", "cache", "probes"} {
		if _, err := os.Stat(filepath.Join(base, kind)); !os.IsNotExist(err) {
			t.Fatal("user namespace changed", kind, err)
		}
	}
}
func TestM3A6CodexAliasStartupAndModelSwitchFailure(t *testing.T) {
	s, base, root, source := a6Setup(t)
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	alias := filepath.Join(filepath.Dir(root), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	t.Chdir(alias)
	before := a4State(t, base, root)
	var out, errOut bytes.Buffer
	payload := `{"hook_event_name":"PreToolUse","session_id":"alias","tool_name":"Bash","tool_input":{"command":"true"}}`
	if code := RunWithInput(context.Background(), []string{"--project-hook", "agenthooks", "run", "--provider=codex", "--owner=style"}, strings.NewReader(payload), &out, &errOut); code != 0 || !strings.Contains(errOut.String(), "canonical project path") {
		t.Fatal(code, &out, &errOut)
	}
	a4AssertState(t, before, base, root)
	t.Chdir(root)
	os.Remove(filepath.Join(scoped.project.Home, "binding.json"))
	before = a4State(t, base, root)
	out.Reset()
	errOut.Reset()
	code := RunWithInput(context.Background(), []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}, strings.NewReader(`{"hook_event_name":"PostModelSwitch","session_id":"switch","model":"new"}`), &out, &errOut)
	if code != 0 || errOut.Len() == 0 {
		t.Fatal(code, &out, &errOut)
	}
	a4AssertState(t, before, base, root)
}
func TestM3A6FrozenRestoresOnlyOriginalBindingOffline(t *testing.T) {
	s, base, root, source := a6Setup(t)
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	check := a5Lock(t, scoped.project.Home)
	a5Offline(t, scoped)
	for _, path := range []string{filepath.Join(root, projectDeclarationPath), filepath.Join(root, projectLauncherPath), filepath.Join(scoped.project.Home, "binding.json"), filepath.Join(scoped.project.Home, "build/index.json")} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := scoped.InstallFrozen(context.Background(), FrozenRequest{Yes: true}); err != nil {
		t.Fatal(err)
	}
	check()
	// The source and detection are still offline; fixing launcher mode is local.
	if err := os.Chmod(filepath.Join(root, projectLauncherPath), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Sync(context.Background(), SyncRequest{}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(root, projectLauncherPath))
	if info.Mode().Perm() != 0o755 {
		t.Fatal(info.Mode())
	}
	moved := filepath.Join(filepath.Dir(root), "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	wrong := a6Scope(t, s, moved, base)
	if _, err := wrong.InstallFrozen(context.Background(), FrozenRequest{Yes: true}); err == nil {
		t.Fatal("moved root inherited frozen binding")
	}
}
