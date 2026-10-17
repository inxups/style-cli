package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter/claude"
)

func a7Setup(t *testing.T) (*Service, string, string, string) {
	requirePOSIXFixture(t, "project/plugin delivery requires POSIX")
	t.Helper()
	s, base, root, source := a6Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.Replace(nativePackageYAML("project-rules"), "rules:", "rules:\n  - {id: static, target: system, text: 'Portable project static.'}", 1))
	scoped := a6Scope(t, s, root, base)
	if _, err := scoped.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	return scoped, base, root, source
}
func a7Export(t *testing.T, s *Service) ExportResult {
	t.Helper()
	result, err := s.Export(context.Background(), ExportRequest{Target: "claude-cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestM3A7PreviewDeterminismAndManifest(t *testing.T) {
	s, base, root, _ := a7Setup(t)
	before := a4State(t, base, root)
	preview, err := s.Export(context.Background(), ExportRequest{Target: "claude-cloud"})
	if err != nil || !preview.AwaitingConfirmation || preview.Changed || len(preview.Native) != 2 || !strings.Contains(preview.Content[".claude/settings.json"], "printf earlier") {
		t.Fatal(preview, err)
	}
	a4AssertState(t, before, base, root)
	if _, err = s.Export(context.Background(), ExportRequest{Target: "claude-cloud", Confirm: func(p ExportResult) (bool, error) {
		p.Content[cloudDir+"/index.json"] = "corrupt"
		p.Manifest.Files[cloudDir+"/index.json"] = "corrupt"
		return true, nil
	}}); err != nil {
		t.Fatal(err)
	}
	m, data, err := readCloud(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.Generation != preview.Manifest.Generation || bytes.Contains(data[cloudDir+"/index.json"], []byte(base)) || bytes.Contains(data[cloudDir+"/explain.json"], []byte(root)) {
		t.Fatal("portable identity changed or leaked")
	}
	again := a7Export(t, s)
	if again.Changed || again.AwaitingConfirmation {
		t.Fatal(again)
	}
	check := a5Lock(t, s.project.Home)
	a7Export(t, s)
	check()
	var report, outErr bytes.Buffer
	code := RunWithService(context.Background(), s, root, []string{"export", "--target=claude-cloud", "--json"}, &report, &outErr)
	if code != 0 || !json.Valid(report.Bytes()) || outErr.Len() != 0 {
		t.Fatal(code, &report, &outErr)
	}
}
func TestM3A7FreshCloneRemoteRuntimeAndIsolation(t *testing.T) {
	s, base, root, _ := a7Setup(t)
	a7Export(t, s)
	clone := filepath.Join(filepath.Dir(root), "cloud clone")
	if err := os.CopyFS(clone, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	t.Chdir(clone)
	t.Setenv("CLAUDE_CODE_REMOTE", "true")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("STYLE_HOME", filepath.Join(t.TempDir(), "absent-local"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "absent-claude"))
	args := []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}
	before := a4State(t, clone, base)
	for _, session := range []string{"one", "two"} {
		var out, errOut bytes.Buffer
		payload := `{"hook_event_name":"PreToolUse","session_id":"` + session + `","cwd":"/untrusted","tool_name":"Bash","tool_input":{"command":"true"}}`
		if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &out, &errOut); code != 0 || !strings.Contains(out.String(), "Be careful") {
			t.Fatal(code, &out, &errOut)
		}
	}
	a4AssertState(t, before, clone, base)
	matches, err := filepath.Glob(filepath.Join(os.Getenv("HOME"), ".style/cloud-state/*/state/*/session.json"))
	if err != nil || len(matches) != 2 {
		t.Fatal(matches, err)
	}
	t.Setenv("CLAUDE_CODE_REMOTE", "false")
	var out, errOut bytes.Buffer
	code := RunWithInput(context.Background(), args, strings.NewReader(`{"hook_event_name":"PreToolUse","session_id":"local","tool_name":"Bash"}`), &out, &errOut)
	if code != 0 || strings.Contains(out.String(), "Be careful") || !strings.Contains(errOut.String(), "no local binding") {
		t.Fatal(code, &out, &errOut)
	}
}
func TestM3A7InputAndBundleFailures(t *testing.T) {
	for _, damage := range []string{"source", "settings", "approval", "index", "launcher", "symlink", "unknown-file", "bad-manifest", "missing-bundle", "runtime-version", "path-escape", "missing-required"} {
		t.Run(damage, func(t *testing.T) {
			s, base, root, _ := a7Setup(t)
			if damage != "source" && damage != "settings" && damage != "approval" && damage != "index" && damage != "launcher" {
				a7Export(t, s)
			}
			file := filepath.Join(root, cloudDir, "index.json")
			switch damage {
			case "source":
				writeSyncFile(t, filepath.Join(s.project.Home, "packages/project-rules/STYLE.yaml"), "invalid")
			case "settings":
				writeSyncFile(t, filepath.Join(s.project.Home, "settings/project-rules.yaml"), "unexpected: text")
			case "approval":
				writeSyncFile(t, approvalPath(s.project.Home, "project-rules"), "{}")
			case "index":
				writeSyncFile(t, filepath.Join(s.project.Home, "build/index.json"), "{}")
			case "launcher":
				writeSyncFile(t, filepath.Join(root, projectLauncherPath), "foreign")
			case "symlink":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(s.project.Home, "build/index.json"), file); err != nil {
					t.Fatal(err)
				}
			case "unknown-file":
				writeSyncFile(t, filepath.Join(root, cloudDir, "foreign.txt"), "keep")
			case "bad-manifest":
				writeSyncFile(t, filepath.Join(root, cloudManifestPath), "{}")
			case "missing-bundle":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			default:
				raw := readSyncFile(t, filepath.Join(root, cloudManifestPath))
				var m CloudManifest
				if json.Unmarshal(raw, &m) != nil {
					t.Fatal("manifest")
				}
				switch damage {
				case "runtime-version":
					m.Runtime = "999.0.0"
				case "path-escape":
					m.Files["../private"] = "sha256:" + strings.Repeat("0", 64)
				case "missing-required":
					delete(m.Files, projectLauncherPath)
				}
				b, _ := json.Marshal(m)
				writeSyncFile(t, filepath.Join(root, cloudManifestPath), string(b))
			}
			before := a4State(t, base, root)
			if _, err := s.Export(context.Background(), ExportRequest{Target: "claude-cloud", Yes: true}); err == nil {
				t.Fatal("accepted damaged installation")
			}
			a4AssertState(t, before, base, root)
		})
	}
}
func TestM3A7RemoteFailuresUseCodecAndNoFallback(t *testing.T) {
	for _, damage := range []string{"digest", "no-bundle", "wrong-root", "home-in-repo", "bad-manifest"} {
		for _, closed := range []bool{false, true} {
			t.Run(damage+map[bool]string{true: "/closed", false: "/open"}[closed], func(t *testing.T) {
				s, base, root, _ := a7Setup(t)
				a7Export(t, s)
				t.Chdir(root)
				t.Setenv("CLAUDE_CODE_REMOTE", "true")
				t.Setenv("HOME", t.TempDir())
				args := []string{"--project-hook", "agenthooks", "run", "--provider=claude-code", "--owner=style"}
				if closed {
					args = append(args, "--fail-closed")
				}
				switch damage {
				case "digest":
					writeSyncFile(t, filepath.Join(root, cloudDir, "index.json"), "{}")
				case "no-bundle":
					if err := os.Remove(filepath.Join(root, cloudManifestPath)); err != nil {
						t.Fatal(err)
					}
				case "wrong-root":
					args = append([]string{"--project-entry-root=/different"}, args...)
				case "home-in-repo":
					t.Setenv("HOME", root)
				case "bad-manifest":
					writeSyncFile(t, filepath.Join(root, cloudManifestPath), "{}")
				}
				before := a4State(t, root, base)
				homeBefore := a4State(t, os.Getenv("HOME"), base)
				var out, errOut bytes.Buffer
				code := RunWithInput(context.Background(), args, strings.NewReader(`{"hook_event_name":"PreToolUse","session_id":"s","tool_name":"Bash"}`), &out, &errOut)
				want := 0
				if closed {
					want = 1
				}
				if code != want || strings.Contains(out.String(), "Be careful") || errOut.Len() == 0 {
					t.Fatal(code, &out, &errOut)
				}
				a4AssertState(t, before, root, base)
				a4AssertState(t, homeBefore, os.Getenv("HOME"), base)
			})
		}
	}
}
func TestM3A7ReexportStaleAndUninstall(t *testing.T) {
	s, _, root, source := a7Setup(t)
	first := a7Export(t, s)
	b := readSyncFile(t, filepath.Join(source, "STYLE.yaml"))
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(string(b), "Be careful", "Changed approved instruction"))
	if _, err := s.Update(context.Background(), UpdateRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	var d DoctorResult
	s.checkCloudExport(s.project.Home, &d)
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Code != "cloud-export-stale" {
		t.Fatal(d)
	}
	next := a7Export(t, s)
	if !next.Changed || first.Manifest.Generation == next.Manifest.Generation {
		t.Fatal(next)
	}
	if _, _, err := readCloud(root); err != nil {
		t.Fatal(err)
	}
	before := a4State(t, s.project.Home, root)
	preview, err := s.Uninstall(context.Background(), UninstallRequest{All: true})
	if err != nil || !preview.AwaitingConfirmation || len(preview.CloudFiles) != 3 {
		t.Fatal(preview, err)
	}
	a4AssertState(t, before, s.project.Home, root)
	if _, err := s.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{cloudManifestPath, cloudDir + "/index.json", cloudDir + "/explain.json", projectDeclarationPath, projectLauncherPath} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal(name, err)
		}
	}
}
func TestM3A7ConfirmationDriftAndCancellation(t *testing.T) {
	for _, damage := range []string{"index", "declaration", "target", "cancel"} {
		t.Run(damage, func(t *testing.T) {
			s, base, root, _ := a7Setup(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result, err := s.Export(ctx, ExportRequest{Target: "claude-cloud", Confirm: func(p ExportResult) (bool, error) {
				switch damage {
				case "index":
					writeSyncFile(t, filepath.Join(s.project.Home, "build/index.json"), "{}")
				case "declaration":
					writeSyncFile(t, filepath.Join(root, projectDeclarationPath), "{}")
				case "target":
					writeSyncFile(t, filepath.Join(root, cloudDir, "foreign"), "keep")
				case "cancel":
					cancel()
				}
				return true, nil
			}})
			if err == nil || result.Changed {
				t.Fatal(result, err)
			}
			if _, e := os.Stat(filepath.Join(root, cloudManifestPath)); !errors.Is(e, os.ErrNotExist) {
				t.Fatal(e)
			}
			if _, e := os.Stat(filepath.Join(base, "cloud-export.json")); !errors.Is(e, os.ErrNotExist) {
				t.Fatal(e)
			}
		})
	}
}
func TestM3A7CLIAndUnsupportedScope(t *testing.T) {
	s, _, root, _ := a7Setup(t)
	for _, args := range [][]string{{"export"}, {"export", "--target=other"}, {"export", "--target"}, {"export", "--target=claude-cloud", "--target=claude-cloud"}, {"export", "--target=claude-cloud", "--unknown"}} {
		var out, errOut bytes.Buffer
		if code := RunWithService(context.Background(), s, root, args, &out, &errOut); code != 1 {
			t.Fatal(args, code)
		}
	}
	plain, err := New(claude.Definition())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plain.Export(context.Background(), ExportRequest{Target: "claude-cloud", Yes: true}); err == nil {
		t.Fatal("user export accepted")
	}
}

func TestM3A7EveryCancellationRestoresFiles(t *testing.T) {
	s, _, _, _ := a7Setup(t)
	count := &a4CancelAt{Context: context.Background(), at: 1000000}
	if _, err := s.Export(count, ExportRequest{Target: "claude-cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	for at := 1; at <= count.checks; at++ {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			s, base, root, _ := a7Setup(t)
			a4OldTimes(t, base, root)
			before := a4State(t, base, root)
			fault := &a4CancelAt{Context: context.Background(), at: at}
			result, err := s.Export(fault, ExportRequest{Target: "claude-cloud", Yes: true})
			if !errors.Is(err, context.Canceled) || result.Changed || !fault.reached {
				t.Fatalf("at=%d result=%+v error=%v checks=%d", at, result, err, fault.checks)
			}
			a4AssertState(t, before, base, root)
		})
	}
}
func TestM3A7MultiplePackagesAndPartialRemoval(t *testing.T) {
	s, base, root, source := a7Setup(t)
	a7Export(t, s)
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second-source"), a4Manifest("second", "Second portable instruction."))
	if _, err := s.Install(context.Background(), InstallRequest{Source: second, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if !a7Export(t, s).Changed {
		t.Fatal("missing reexport")
	}
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Name: "project-rules", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, cloudManifestPath)); err != nil {
		t.Fatal("partial removal deleted bundle", err)
	}
	if !a7Export(t, s).Changed {
		t.Fatal("partial reexport")
	}
	writeSyncFile(t, filepath.Join(root, cloudDir, "index.json"), "modified export")
	before := a4State(t, base, root)
	if _, err := s.Uninstall(context.Background(), UninstallRequest{All: true, Yes: true}); err == nil {
		t.Fatal("deleted modified export")
	}
	a4AssertState(t, before, base, root)
}
