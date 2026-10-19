package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/probe"
	"github.com/speakeasy-api/agenthooks"
)

func a6ProjectGolden(t *testing.T, name, output string, s *Service, root string) {
	t.Helper()
	output = strings.ReplaceAll(output, s.project.Home, "$PROJECT_HOME")
	canonical, _ := canonicalProjectPath(root)
	for _, path := range []string{canonical, root} {
		output = strings.ReplaceAll(output, path, "$FIXTURE")
	}
	if strings.HasPrefix(name, "stats.") {
		output = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`).ReplaceAllLiteralString(output, "$TIME")
	}
	if strings.HasSuffix(name, ".txt") {
		lines := strings.Split(output, "\n")
		for i := range lines {
			lines[i] = strings.TrimRight(lines[i], " \t")
		}
		output = strings.Join(lines, "\n")
	}
	path := filepath.Join("..", "..", "testdata", "project", name)
	if os.Getenv("STYLE_M3_A6_RECORD") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if string(readSyncFile(t, path)) != output {
		t.Fatalf("%s differs\n%s", name, output)
	}
}
func TestM3A6CommandGolden(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonMode], func(t *testing.T) {
			s, base, root, source := a6Setup(t)
			scoped := a6Scope(t, s, root, base)
			suffix := "txt"
			if jsonMode {
				suffix = "json"
			}
			for _, phase := range []string{"plan", "preview", "apply", "list", "sync", "trust", "update", "stats", "uninstall", "error"} {
				args := []string{}
				switch phase {
				case "plan":
					args = []string{"plan", source}
				case "preview":
					args = []string{"install", source}
				case "apply":
					args = []string{"install", source, "--yes"}
				case "update":
					args = []string{"update", "--all", "--yes"}
				case "trust":
					args = []string{"trust", "project-rules", "--yes"}
				case "uninstall":
					args = []string{"uninstall", "--all", "--yes"}
				case "error":
					args = []string{"list", "--project="}
				default:
					args = []string{phase}
				}
				if phase != "error" {
					args = append(args, "--project=.")
				}
				if jsonMode {
					args = append(args, "--json")
				}
				before := a4State(t, base, root)
				var out, errOut bytes.Buffer
				code := RunWithService(context.Background(), s, root, args, &out, &errOut)
				want := 0
				if phase == "preview" || phase == "error" {
					want = 1
				}
				if code != want {
					t.Fatal(phase, code, &out, &errOut)
				}
				if phase == "preview" || phase == "error" || phase == "plan" || phase == "list" || phase == "stats" {
					a4AssertState(t, before, base, root)
				}
				if jsonMode {
					dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
					var value any
					if dec.Decode(&value) != nil || dec.Decode(new(any)) != io.EOF {
						t.Fatal("not one JSON", &out)
					}
				}
				a6ProjectGolden(t, phase+".stdout."+suffix, out.String(), scoped, filepath.Dir(root))
				a6ProjectGolden(t, phase+".stderr."+suffix, errOut.String(), scoped, filepath.Dir(root))
			}
		})
	}
}
func TestM3A6ProbeUsesProjectBindings(t *testing.T) {
	s, base, root, source := a6Setup(t)
	scoped := a6Scope(t, s, root, base)
	a6Install(t, scoped, source)
	for _, config := range []string{os.Getenv("CODEX_HOME"), os.Getenv("CLAUDE_CONFIG_DIR")} {
		if err := os.MkdirAll(config, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	f := &a2Fixture{home: scoped.project.Home, root: root, runtime: exe, codexExe: exe, claudeExe: exe, now: now}
	deps := defaultProbeDependencies()
	deps.now = func() time.Time { return f.now }
	deps.lookPath = func(string) (string, error) { return exe, nil }
	deps.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
		if pid == os.Getpid() {
			return agenthooks.ProcessIdentity{PID: pid, Birth: "owner"}, nil
		}
		if pid == 400 || pid == 401 {
			id, err := agenthooks.ExecutableIdentity(exe)
			return agenthooks.ProcessIdentity{PID: pid, ParentPID: 1, Birth: "harness", Executable: exe, ExecutableID: id}, err
		}
		return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessGone
	}
	deps.poll = func(ctx context.Context) error { f.now = f.now.Add(2 * time.Minute); return ctx.Err() }
	scoped.probeDeps = &deps
	result, err := scoped.Probe(context.Background(), ProbeRequest{Timeout: time.Second, Ready: func(ProbeReady) error {
		for _, id := range []string{"codex", "claude-code"} {
			f.emit(t, id, func(receipt *probe.Receipt) { receipt.Processes[1].Birth = "harness" })
		}
		return nil
	}})
	if err != nil || result.Status != "observed" || len(result.Targets) != 2 {
		t.Fatal(result, err)
	}
	a2NoLeases(t, scoped.project.Home)
	a2NoLeases(t, base)
	if _, err = os.Stat(filepath.Join(base, "probes")); !os.IsNotExist(err) {
		t.Fatal("probe leaked into user scope", err)
	}
	for _, target := range result.Targets {
		if !target.CallbackObserved || !strings.Contains(target.Route, "project") {
			t.Fatal(target)
		}
	}
}
