package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The command matrix is the CLI contract in one table: every command, its
// usage errors, rejected requests, declined confirmations and successes, in
// text and --json, in user and project scope. Each row runs on a fresh
// fixture; checkMatrixCell holds the rules every row must satisfy.

type matrixFixture struct {
	service *Service
	vars    map[string]string // $SRC, $NEW, $WORK, $HOME, $ROOT
}

type matrixCase struct {
	name    string
	fixture string // "user" (copied package "first"), "linked", "project"
	args    []string
	input   string
	setup   func(t *testing.T, f matrixFixture)
	exit    int
	code    string // expected JSON diagnostic code when the command fails
	noJSON  bool
	pending string // "text" or "json": a mode whose fix has not landed yet
}

func newMatrixFixture(t *testing.T, kind string) matrixFixture {
	t.Helper()
	work := t.TempDir()
	payload := `{"session_id":"matrix","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"true"},"cwd":"/offline"}`
	for name, data := range map[string]string{"payload.json": payload, "text.txt": "hello world\n"} {
		if err := os.WriteFile(filepath.Join(work, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(work, "exists"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "exists", "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"$WORK": work, "$NEW": writeInstallPackage(t, filepath.Join(work, "second"), a4Manifest("second", "Second."))}
	if kind == "project" {
		s, base, root, source := a6Setup(t)
		a6Install(t, a6Scope(t, s, root, base), source)
		vars["$SRC"], vars["$ROOT"], vars["$HOME"] = source, root, base
		return matrixFixture{s, vars}
	}
	s, home, _, source := a4Setup(t, "codex-user", a4Manifest("first", "Before."), kind == "linked")
	t.Setenv("STYLE_HOME", home)
	vars["$SRC"], vars["$HOME"] = source, home
	return matrixFixture{s, vars}
}

func (f matrixFixture) expand(args []string) []string {
	expanded := make([]string, len(args))
	for i, arg := range args {
		for name, value := range f.vars {
			arg = strings.ReplaceAll(arg, name, value)
		}
		expanded[i] = arg
	}
	return expanded
}

func writeMatrixFile(path, data string) func(*testing.T, matrixFixture) {
	return func(t *testing.T, f matrixFixture) {
		if err := os.WriteFile(f.expand([]string{path})[0], []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

var (
	brokenLock    = writeMatrixFile("$HOME/style.lock.yaml", "{ not a lock")
	changedSource = writeMatrixFile("$SRC/STYLE.yaml", a4Manifest("first", "After."))
	staleBuild    = writeMatrixFile("$HOME/build/explain.json", "corrupt derived file\n")
	emptyHome     = func(t *testing.T, f matrixFixture) { t.Setenv("STYLE_HOME", filepath.Join(t.TempDir(), "home")) }
)

var commandMatrix = []matrixCase{
	{name: "unknown command", args: []string{"bogus"}, exit: ExitUsage, noJSON: true},
	{name: "help unknown", args: []string{"help", "bogus"}, exit: ExitUsage, noJSON: true},

	{name: "init extra arg", args: []string{"init", "a", "b"}, exit: ExitUsage, noJSON: true},
	{name: "init malformed name", args: []string{"init", "Bad_Name"}, exit: ExitUsage, noJSON: true},
	{name: "init existing", args: []string{"init", "exists"}, exit: ExitInvalid, noJSON: true},
	{name: "init ok", args: []string{"init", "fresh"}, exit: ExitOK, noJSON: true},

	{name: "validate extra arg", args: []string{"validate", "a", "b"}, exit: ExitUsage},
	{name: "validate malformed git", args: []string{"validate", "git+https://example.invalid/r.git?ref=main"}, exit: ExitUsage},
	{name: "validate missing", args: []string{"validate", "$WORK/missing"}, exit: ExitInvalid, code: "invalid-package"},
	{name: "validate ok", args: []string{"validate", "$SRC"}, exit: ExitOK},

	{name: "count no file", args: []string{"count"}, exit: ExitUsage},
	{name: "count missing", args: []string{"count", "$WORK/missing.txt"}, exit: ExitInvalid},
	{name: "count ok", args: []string{"count", "$WORK/text.txt"}, exit: ExitOK},

	{name: "plan unknown harness", args: []string{"plan", "$SRC", "--harness", "bogus"}, exit: ExitUsage},
	{name: "plan missing", args: []string{"plan", "$WORK/missing"}, exit: ExitInvalid, code: "invalid-package"},
	{name: "plan ok", args: []string{"plan", "$SRC", "--harness", "codex"}, exit: ExitOK},

	{name: "install no source", args: []string{"install"}, exit: ExitUsage},
	{name: "install unknown route", args: []string{"install", "$NEW", "--route", "bogus=x"}, exit: ExitUsage},
	{name: "install unknown setting", args: []string{"install", "$NEW", "--harness", "codex", "--set", "unknown=v"}, input: "y\n", exit: ExitInvalid, code: "request-rejected"},
	{name: "install declined", args: []string{"install", "$NEW", "--harness", "codex"}, input: "n\n", exit: ExitInvalid, code: "confirmation-declined"},
	{name: "install over budget", args: []string{"install", "$NEW", "--harness", "codex"}, input: "y\n", exit: ExitBlocked,
		setup: writeMatrixFile("$NEW/STYLE.yaml", a6LongManifest("second", "Second.", "budget: {per_event: 1, on: error}\n", "tool"))},
	{name: "install ok", args: []string{"install", "$NEW", "--harness", "codex"}, input: "y\n", exit: ExitOK},

	{name: "restore extra arg", args: []string{"restore", "x"}, exit: ExitUsage},
	{name: "restore no lock", args: []string{"restore"}, setup: emptyHome, exit: ExitInvalid, code: "frozen-metadata-incomplete"},
	{name: "restore declined", args: []string{"restore"}, setup: staleBuild, input: "n\n", exit: ExitInvalid, code: "confirmation-declined"},
	{name: "restore ok", args: []string{"restore"}, setup: staleBuild, input: "y\n", exit: ExitOK},
	{name: "restore broken lock", args: []string{"restore"}, setup: brokenLock, exit: ExitInvalid},

	{name: "update no name", args: []string{"update"}, exit: ExitUsage},
	{name: "update malformed ref", args: []string{"update", "first", "--ref", "main"}, exit: ExitUsage},
	{name: "update missing", args: []string{"update", "missing"}, exit: ExitInvalid, code: "request-rejected"},
	{name: "update declined", args: []string{"update", "first"}, setup: changedSource, input: "n\n", exit: ExitInvalid, code: "confirmation-declined"},
	{name: "update ok", args: []string{"update", "first"}, setup: changedSource, input: "y\n", exit: ExitOK},
	{name: "update broken lock", args: []string{"update", "first"}, setup: brokenLock, exit: ExitInvalid, code: "lock-invalid"},

	{name: "uninstall no name", args: []string{"uninstall"}, exit: ExitUsage},
	{name: "uninstall missing", args: []string{"uninstall", "missing"}, input: "y\n", exit: ExitInvalid, code: "request-rejected"},
	{name: "uninstall declined", args: []string{"uninstall", "first"}, input: "n\n", exit: ExitInvalid, code: "confirmation-declined"},
	{name: "uninstall ok", args: []string{"uninstall", "first"}, input: "y\n", exit: ExitOK},
	{name: "uninstall broken lock", args: []string{"uninstall", "first"}, setup: brokenLock, input: "y\n", exit: ExitInvalid, code: "lock-invalid"},

	{name: "sync extra arg", args: []string{"sync", "x"}, exit: ExitUsage},
	{name: "sync ok", args: []string{"sync"}, exit: ExitOK},
	{name: "sync needs trust", fixture: "linked", args: []string{"sync"}, setup: changedSource, exit: ExitSyncChanged},
	{name: "sync broken lock", args: []string{"sync"}, setup: brokenLock, exit: ExitInvalid, code: "lock-invalid"},

	{name: "trust no name", args: []string{"trust"}, exit: ExitUsage},
	{name: "trust missing", args: []string{"trust", "missing"}, input: "y\n", exit: ExitInvalid, code: "request-rejected"},
	{name: "trust declined", fixture: "linked", args: []string{"trust", "first"}, setup: changedSource, input: "n\n", exit: ExitInvalid, code: "confirmation-declined"},
	{name: "trust ok", fixture: "linked", args: []string{"trust", "first"}, setup: changedSource, input: "y\n", exit: ExitOK},
	{name: "trust broken lock", args: []string{"trust", "first"}, setup: brokenLock, input: "y\n", exit: ExitInvalid, code: "lock-invalid"},

	{name: "list extra arg", args: []string{"list", "x"}, exit: ExitUsage},
	{name: "list ok", args: []string{"list"}, exit: ExitOK},
	{name: "list broken lock", args: []string{"list"}, setup: brokenLock, exit: ExitInvalid, code: "lock-invalid"},

	{name: "doctor extra arg", args: []string{"doctor", "x"}, exit: ExitUsage},
	{name: "doctor ok", args: []string{"doctor"}, exit: ExitOK},
	{name: "doctor broken lock", args: []string{"doctor"}, setup: brokenLock, exit: ExitInvalid, code: "lock-invalid"},

	{name: "stats unknown by", args: []string{"stats", "--by", "bogus"}, exit: ExitUsage},
	{name: "stats malformed since", args: []string{"stats", "--since", "bogus"}, exit: ExitUsage},
	{name: "stats ok", args: []string{"stats"}, exit: ExitOK},

	{name: "explain no harness", args: []string{"explain", "--payload", "$WORK/payload.json"}, exit: ExitUsage},
	{name: "explain auto harness", args: []string{"explain", "--harness", "auto", "--payload", "$WORK/payload.json"}, exit: ExitUsage},
	{name: "explain missing payload", args: []string{"explain", "--harness", "codex", "--payload", "$WORK/missing.json"}, exit: ExitInvalid},
	{name: "explain ok", args: []string{"explain", "--harness", "codex", "--payload", "$WORK/payload.json"}, exit: ExitOK},

	{name: "probe unknown harness", args: []string{"probe", "--harness", "bogus"}, exit: ExitUsage},
	{name: "probe malformed timeout", args: []string{"probe", "--timeout", "bogus"}, exit: ExitUsage},
	{name: "probe not installed", args: []string{"probe", "--harness", "claude-code"}, exit: ExitInvalid},

	{name: "project missing value", fixture: "project", args: []string{"list", "--project"}, exit: ExitUsage},
	{name: "project unsupported", fixture: "project", args: []string{"init", "x", "--project", "$ROOT"}, exit: ExitUsage, noJSON: true},
	{name: "project unbound", fixture: "project", args: []string{"list", "--project", "$WORK/missing"}, exit: ExitInvalid, code: "project-binding-invalid"},
	{name: "project list ok", fixture: "project", args: []string{"list", "--project", "$ROOT"}, exit: ExitOK},
	{name: "project trust missing", fixture: "project", args: []string{"trust", "missing", "--project", "$ROOT"}, input: "y\n", exit: ExitInvalid, code: "request-rejected"},
	{name: "project uninstall missing", fixture: "project", args: []string{"uninstall", "missing", "--project", "$ROOT"}, input: "y\n", exit: ExitInvalid, code: "request-rejected"},
	{name: "project uninstall declined", fixture: "project", args: []string{"uninstall", "--all", "--project", "$ROOT"}, input: "n\n", exit: ExitInvalid, code: "confirmation-declined"},
}

func TestCommandMatrix(t *testing.T) {
	requirePOSIXFixture(t, "the matrix uses POSIX project and native fixtures")
	for _, row := range commandMatrix {
		for _, jsonMode := range []bool{false, true} {
			if jsonMode && row.noJSON || row.pending == map[bool]string{false: "text", true: "json"}[jsonMode] {
				continue
			}
			t.Run(row.name+map[bool]string{false: "/text", true: "/json"}[jsonMode], func(t *testing.T) {
				kind := row.fixture
				if kind == "" {
					kind = "user"
				}
				f := newMatrixFixture(t, kind)
				if row.setup != nil {
					row.setup(t, f)
				}
				args := f.expand(row.args)
				if jsonMode {
					args = append(args, "--json")
				}
				work := f.vars["$WORK"]
				var out, errOut bytes.Buffer
				exit := RunWithServiceInput(context.Background(), f.service, work, args, strings.NewReader(row.input), &out, &errOut)
				checkMatrixCell(t, row, args, jsonMode, exit, out.String(), errOut.String())
			})
		}
	}
}

func checkMatrixCell(t *testing.T, row matrixCase, args []string, jsonMode bool, exit int, stdout, stderr string) {
	t.Helper()
	command := args[0]
	report := func(format string, values ...any) {
		t.Errorf(format+"\nexit=%d\nstdout=%s\nstderr=%s", append(values, exit, stdout, stderr)...)
	}
	if exit != row.exit {
		report("exit = %d, want %d", exit, row.exit)
		return
	}
	if strings.Contains(stdout+stderr, "app: ") {
		report("internal package prefix in output")
	}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "style ") && !strings.HasPrefix(line, "style "+command+":") && !strings.HasPrefix(line, "style "+command+" ") {
			report("stderr line names another command: %q", line)
		}
	}
	if exit == ExitUsage {
		if stdout != "" {
			report("usage error wrote stdout")
		}
		if !strings.HasPrefix(stderr, "style "+command+": ") && !strings.HasPrefix(stderr, "style: unknown command") {
			report("usage error does not start with the command name")
		}
		return
	}
	if !jsonMode {
		if exit != ExitOK && exit != ExitSyncChanged && strings.TrimSpace(stderr) == "" {
			report("text failure wrote nothing to stderr")
		}
		return
	}
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var envelope struct {
		Format      int
		Command     string
		Status      string
		Scope       map[string]any
		Diagnostics []Diagnostic
		Result      json.RawMessage
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		report("stdout is not JSON: %v", err)
		return
	}
	if decoder.Decode(new(any)) != nil || decoder.Decode(new(any)) != io.EOF {
		report("stdout is not exactly one JSON document")
	}
	if keys := objectKeys(t, []byte(stdout)); !reflect.DeepEqual(keys, []string{"format", "command", "status", "scope", "diagnostics", "result"}) {
		report("envelope keys = %v", objectKeys(t, []byte(stdout)))
	}
	if envelope.Command != command || envelope.Status != envelopeStatus(exit) {
		report("command=%q status=%q", envelope.Command, envelope.Status)
	}
	wantKind := "user"
	if row.fixture == "project" {
		wantKind = "project"
	}
	if envelope.Scope["kind"] != wantKind {
		report("scope = %v", envelope.Scope)
	}
	if string(envelope.Result) == "{}" {
		report("empty result object")
	}
	if exit == ExitOK || exit == ExitSyncChanged {
		return
	}
	if len(envelope.Diagnostics) == 0 {
		report("failure without diagnostics")
	}
	found := row.code == ""
	for _, diagnostic := range envelope.Diagnostics {
		if diagnostic.Code == "" || diagnostic.Severity == "" || diagnostic.Message == "" {
			report("incomplete diagnostic %+v", diagnostic)
		}
		found = found || diagnostic.Code == row.code
	}
	if !found {
		report("diagnostics %+v lack code %q", envelope.Diagnostics, row.code)
	}
	t.Logf("codes: %v", diagnosticCodes(envelope.Diagnostics))
}

func diagnosticCodes(diagnostics []Diagnostic) []string {
	codes := make([]string, len(diagnostics))
	for i, diagnostic := range diagnostics {
		codes[i] = diagnostic.Code
	}
	return codes
}

// Harnesses treat exit 2 from a hook as a block, so malformed hook command
// lines must fail with 1 instead.
func TestHookInvocationsNeverExitUsage(t *testing.T) {
	for _, args := range [][]string{
		{"agenthooks"},
		{"agenthooks", "bogus"},
		{"agenthooks", "run"},
		{"agenthooks", "run", "--provider=bogus"},
		{"agenthooks", "run", "--provider=codex", "--provider=claude-code"},
		{"agenthooks", "run", "--provider=codex", "--fail-closed=maybe"},
		{"--style-home=" + t.TempDir(), "agenthooks", "run", "--provider=codex"},
	} {
		var out, errOut bytes.Buffer
		if exit := RunWithInput(context.Background(), args, strings.NewReader("{}"), &out, &errOut); exit == ExitUsage {
			t.Errorf("%v exit = 2; stderr=%s", args, &errOut)
		}
	}
}

// Each --project-source usage error names the one condition that failed.
func TestProjectSourceUsageNamesOneCondition(t *testing.T) {
	s, base, root, source := a6Setup(t)
	many := []string{"install", source, "--project", root}
	for range 64 {
		many = append(many, "--project-source", source)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"install", source, "--project-source", source}, "--project-source requires --project\n"},
		{[]string{"install", source, "--project", root, "--project-source", source, "--link"}, "--project-source cannot be combined with --link\n"},
		{many, "--project-source accepts at most 63 sources (64 packages with the main source)\n"},
		{[]string{"install", source, "--project", root, "--project-set", "x.k=v"}, "--project-set requires --project-source\n"},
	} {
		var out, errOut bytes.Buffer
		exit := RunWithServiceInput(context.Background(), s, base, test.args, strings.NewReader(""), &out, &errOut)
		if exit != ExitUsage || !strings.HasPrefix(errOut.String(), "style install: "+test.want) {
			t.Errorf("%v: exit=%d stdout=%q stderr=%q", test.args[2:min(len(test.args), 6)], exit, &out, &errOut)
		}
	}
}
