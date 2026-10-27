package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidateAndPlanTextAreStable(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	if code := RunWithService(context.Background(), service, root, []string{"init", "demo"}, &bytes.Buffer{}, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("init exit = %d", code)
	}
	var validateOut, validateErr bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"validate", "demo"}, &validateOut, &validateErr); code != ExitOK {
		t.Fatalf("validate exit = %d, stdout=%s stderr=%s", code, validateOut.String(), validateErr.String())
	}
	if got, want := validateOut.String(), "valid\n"; got != want {
		t.Fatalf("validate output = %q, want %q", got, want)
	}
	var first, second bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"plan", "demo", "--harness", "codex"}, &first, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("plan exit = %d, output=%s", code, first.String())
	}
	if code := RunWithService(context.Background(), service, root, []string{"plan", "demo", "--harness", "codex"}, &second, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("second plan exit = %d", code)
	}
	if first.String() != second.String() {
		t.Fatalf("plan output is not stable:\n%s\n---\n%s", first.String(), second.String())
	}
	for _, marker := range []string{"Harness: codex", "Token (estimated", "style: native", "review: native"} {
		if !strings.Contains(first.String(), marker) {
			t.Errorf("plan output missing %q: %s", marker, first.String())
		}
	}
}

func TestRunValidateJSONIncludesStableCodesAndExitStatus(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	manifest := "spec: style/v0.3\nname: bad\nversion: 0.1.0\ndescription: bad\nrules:\n  - id: duplicate\n    target: system\n    text: one\n  - id: duplicate\n    target: system\n    text: two\n"
	if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	var output, errorsOut bytes.Buffer
	code := RunWithService(context.Background(), service, root, []string{"validate", ".", "--json"}, &output, &errorsOut)
	if code != ExitInvalid {
		t.Fatalf("validate exit = %d, stdout=%s stderr=%s", code, output.String(), errorsOut.String())
	}
	var diagnostics []Diagnostic
	if err := decodeReport(output.Bytes(), &diagnostics); err != nil {
		t.Fatalf("validate JSON = %v: %s", err, output.String())
	}
	if len(diagnostics) == 0 || diagnostics[0].Code != "dup-id" {
		t.Fatalf("diagnostics = %#v, want dup-id first", diagnostics)
	}
	if !strings.HasPrefix(errorsOut.String(), "style validate: dup-id ") {
		t.Fatalf("unexpected stderr: %s", errorsOut.String())
	}
}

func TestRunPlanJSONAndArgumentErrors(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	if code := RunWithService(context.Background(), service, root, []string{"init", "demo"}, &bytes.Buffer{}, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("init exit = %d", code)
	}
	var output bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"plan", "demo", "--harness=codex", "--json", "--assume-requests=10", "--assume-triggers=2"}, &output, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("plan JSON exit = %d: %s", code, output.String())
	}
	var report PlanResult
	if err := decodeReport(output.Bytes(), &report); err != nil {
		t.Fatalf("plan JSON = %v: %s", err, output.String())
	}
	if report.Package != "demo" || len(report.Reports) != 1 || report.Reports[0].Provider != "codex" {
		t.Fatalf("plan report = %#v", report)
	}
	if got := report.Reports[0].SessionCost; got.Requests != 10 || got.Triggers != 2 {
		t.Fatalf("session assumptions = %#v, want requests=10 triggers=2", got)
	}
	var implicit, explicit bytes.Buffer
	if code := RunWithService(context.Background(), service, filepath.Join(root, "demo"), []string{"plan", "--harness=codex", "--json"}, &implicit, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("plan without source exit = %d: %s", code, implicit.String())
	}
	if code := RunWithService(context.Background(), service, filepath.Join(root, "demo"), []string{"plan", ".", "--harness=codex", "--json"}, &explicit, &bytes.Buffer{}); code != ExitOK || implicit.String() != explicit.String() {
		t.Fatalf("plan without source = %s, plan . = %s", implicit.String(), explicit.String())
	}
	var errOut bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"plan", "demo", "--harness", "missing"}, &bytes.Buffer{}, &errOut); code != ExitUsage {
		t.Fatalf("unknown harness exit = %d, stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "unknown harness") {
		t.Fatalf("unknown harness stderr = %q", errOut.String())
	}
	if code := RunWithService(context.Background(), service, root, []string{"validate", "--unknown"}, &bytes.Buffer{}, &errOut); code != ExitUsage {
		t.Fatalf("unknown validate option exit = %d", code)
	}
}

func TestPlanSelectsMultipleProvidersInCanonicalOrder(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	if code := RunWithService(context.Background(), service, root, []string{"init", "demo"}, &bytes.Buffer{}, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("init exit = %d", code)
	}
	result, err := service.Plan(context.Background(), PlanRequest{Source: filepath.Join(root, "demo"), Harnesses: []string{"claude-code,codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Reports) != 2 || result.Reports[0].Provider != "codex" || result.Reports[1].Provider != "claude-code" {
		t.Fatalf("provider reports = %#v, want codex then claude-code", result.Reports)
	}
	if result.Reports[0].Route != "codex-plugin" || result.Reports[1].Route != "claude-user" {
		t.Fatalf("preferred routes = %q, %q", result.Reports[0].Route, result.Reports[1].Route)
	}
}

func TestCommandsShowHelpAndUseStableFailureExitCodes(t *testing.T) {
	service := testService(t)
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"validate", "--help"}, want: "style validate [source] [--json]\n"},
		{args: []string{"plan", "--help"}, want: "style plan [source] [--harness auto|h[,h]]... [--exact openai:<encoding>|anthropic:<model>] [--json]\n  advanced: [--assume-requests N] [--assume-triggers N]\n  global, before the command: [--project dir] [--style-home dir]\n"},
	} {
		var output, errOutput bytes.Buffer
		if code := RunWithService(context.Background(), service, t.TempDir(), test.args, &output, &errOutput); code != ExitOK {
			t.Fatalf("%v exit = %d stderr=%s", test.args, code, errOutput.String())
		}
		if output.String() != test.want || errOutput.Len() != 0 {
			t.Fatalf("%v output=%q stderr=%q, want %q", test.args, output.String(), errOutput.String(), test.want)
		}
	}
	if ExitOK != 0 || ExitInvalid != 1 || ExitUsage != 2 || ExitConflict != 1 || ExitBlocked != 4 {
		t.Fatalf("unexpected public exit code mapping: ok=%d invalid=%d usage=%d conflict=%d blocked=%d", ExitOK, ExitInvalid, ExitUsage, ExitConflict, ExitBlocked)
	}
}

func TestVersionPrintsTheRelease(t *testing.T) {
	var out, errOut bytes.Buffer
	if exit := RunWithInput(context.Background(), []string{"--version"}, strings.NewReader(""), &out, &errOut); exit != ExitOK || out.String() != "style 0.3.2\n" || errOut.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exit, &out, &errOut)
	}
}
