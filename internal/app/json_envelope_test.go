package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJSONEnvelopeWrapsEveryReport(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("STYLE_HOME", home)
	if code := RunWithService(context.Background(), service, root, []string{"init", "demo"}, &bytes.Buffer{}, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("init exit = %d", code)
	}
	for _, test := range []struct {
		args       []string
		code       int
		status     string
		diagnostic string
	}{
		{[]string{"validate", "demo", "--json"}, ExitOK, "ok", ""},
		{[]string{"list", "--json"}, ExitOK, "ok", ""},
		{[]string{"plan", "demo", "--harness=codex", "--json"}, ExitOK, "ok", ""},
		{[]string{"trust", "missing", "--json"}, ExitInvalid, "error", "request-rejected"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := RunWithService(context.Background(), service, root, test.args, &out, &errOut)
			if code != test.code {
				t.Fatalf("exit = %d, stderr=%s", code, &errOut)
			}
			decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
			var keys []string
			if _, err := decoder.Token(); err != nil {
				t.Fatal(err)
			}
			for decoder.More() {
				token, _ := decoder.Token()
				keys = append(keys, token.(string))
				var skip json.RawMessage
				if err := decoder.Decode(&skip); err != nil {
					t.Fatal(err)
				}
			}
			if want := []string{"format", "command", "status", "scope", "diagnostics", "result"}; !reflect.DeepEqual(keys, want) {
				t.Fatalf("keys = %v, want %v", keys, want)
			}
			var envelope jsonEnvelope
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Format != envelopeFormat || envelope.Command != test.args[0] || envelope.Status != test.status || !reflect.DeepEqual(envelope.Scope, map[string]any{"kind": "user", "home": home}) {
				t.Fatalf("envelope = %+v", envelope)
			}
			var diagnostics []Diagnostic
			if err := decodeReport(out.Bytes(), &diagnostics); err != nil {
				t.Fatal(err)
			}
			if test.diagnostic == "" && len(diagnostics) != 0 || test.diagnostic != "" && (len(diagnostics) != 1 || diagnostics[0].Code != test.diagnostic || !strings.Contains(errOut.String(), diagnostics[0].Message)) {
				t.Fatalf("diagnostics = %+v, stderr=%s", diagnostics, &errOut)
			}
			if bytes.Contains(envelope.Result, []byte(`"diagnostics"`)) && !strings.HasPrefix(test.args[0], "plan") {
				t.Fatalf("diagnostics left in result: %s", envelope.Result)
			}
		})
	}
	for _, args := range [][]string{{"list", "--bogus", "--json"}, {"export", "--json"}, {"list", "--project", "--json"}} {
		var out, errOut bytes.Buffer
		if code := RunWithService(context.Background(), service, root, args, &out, &errOut); code != ExitUsage || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("%v exit=%d stdout=%q", args, code, out.String())
		}
	}
}

func TestEnvelopeScopeKeyOrder(t *testing.T) {
	root := t.TempDir()
	service := testService(t)
	service.project = &projectScope{Root: root, Base: filepath.Join(root, "base"), Home: filepath.Join(root, "home")}
	data, err := json.Marshal(service.jsonScope())
	if err != nil {
		t.Fatal(err)
	}
	if keys := objectKeys(t, data); !reflect.DeepEqual(keys, []string{"kind", "root", "base_home", "home", "external_prerequisites", "coexisting_user_packages", "budget_scope"}) {
		t.Fatalf("project scope keys = %v", keys)
	}
	service.project = nil
	t.Setenv("STYLE_HOME", root)
	home, _ := json.Marshal(root)
	if data, _ = json.Marshal(service.jsonScope()); string(data) != `{"kind":"user","home":`+string(home)+`}` {
		t.Fatalf("user scope = %s", data)
	}
}

// objectKeys returns a JSON object's member names in document order.
func objectKeys(t *testing.T, data []byte) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	if _, err := decoder.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for decoder.More() {
		token, _ := decoder.Token()
		keys = append(keys, token.(string))
		var skip json.RawMessage
		if err := decoder.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestWrapReportFallsBackToStderrFailures(t *testing.T) {
	envelope, err := wrapReport("export", ExitInvalid, envelopeScope{Kind: "user"}, nil, []string{"disk full"})
	if err != nil || len(envelope.Diagnostics) != 1 || !bytes.Contains(envelope.Diagnostics[0], []byte(`"code":"command-failed"`)) || string(envelope.Result) != "null" {
		t.Fatalf("envelope = %+v, %v", envelope, err)
	}
	envelope, _ = wrapReport("export", ExitOK, envelopeScope{Kind: "user"}, nil, []string{"ignored"})
	if len(envelope.Diagnostics) != 0 {
		t.Fatalf("successful command diagnostics = %s", envelope.Diagnostics)
	}
}

func TestEnvelopeStatusMirrorsExitCode(t *testing.T) {
	for code, want := range map[int]string{ExitOK: "ok", ExitInvalid: "error", ExitSyncChanged: "needs-trust", ExitBlocked: "blocked"} {
		if got := envelopeStatus(code); got != want {
			t.Fatalf("status(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestSplitDiagnosticsKeepsMemberOrder(t *testing.T) {
	result, diagnostics, err := splitDiagnostics([]byte(`{"z":1,"diagnostics":[{"code":"a"}],"a":{"diagnostics":[]}}`))
	if err != nil || string(result) != `{"z":1,"a":{"diagnostics":[]}}` || len(diagnostics) != 1 || string(diagnostics[0]) != `{"code":"a"}` {
		t.Fatalf("result=%s diagnostics=%s err=%v", result, diagnostics, err)
	}
	result, diagnostics, err = splitDiagnostics([]byte(`[{"code":"b"}]`))
	if err != nil || string(result) != "null" || len(diagnostics) != 1 {
		t.Fatalf("array result=%s diagnostics=%s err=%v", result, diagnostics, err)
	}
	result, diagnostics, err = splitDiagnostics([]byte(`{"diagnostics":[{"code":"c"}]}`))
	if err != nil || string(result) != "null" || len(diagnostics) != 1 {
		t.Fatalf("diagnostics-only result=%s diagnostics=%s err=%v", result, diagnostics, err)
	}
}

func TestUsageAndRejectedErrorsExitCodes(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	t.Setenv("STYLE_HOME", filepath.Join(root, "home"))
	if code := RunWithService(context.Background(), service, root, []string{"init", "demo"}, &bytes.Buffer{}, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("init exit = %d", code)
	}
	for _, test := range []struct {
		args []string
		code int
	}{
		{[]string{"install", "demo", "--route", "bogus=x"}, ExitUsage},
		{[]string{"install", "demo", "--harness", "codex", "--route", "codex=bogus"}, ExitUsage},
		{[]string{"install", "demo", "--project-set", "demo.k=v"}, ExitUsage},
		{[]string{"trust", "missing"}, ExitInvalid},
		{[]string{"install", "demo", "--harness", "codex", "--set", "unknown=v"}, ExitInvalid},
	} {
		var out, errOut bytes.Buffer
		if code := RunWithServiceInput(context.Background(), service, root, test.args, strings.NewReader("y\n"), &out, &errOut); code != test.code {
			t.Fatalf("%v exit = %d, want %d; stderr=%s", test.args, code, test.code, &errOut)
		}
	}
}

func TestJSONFailuresKeepTheirCodes(t *testing.T) {
	service := testService(t)
	root := t.TempDir()
	t.Setenv("STYLE_HOME", filepath.Join(root, "home"))
	if code := RunWithService(context.Background(), service, root, []string{"init", "demo"}, &bytes.Buffer{}, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("init exit = %d", code)
	}
	for _, test := range []struct {
		args []string
		code string
	}{
		{[]string{"install", "git+https://example.invalid/repo.git?x=1", "--json"}, "source-invalid"},
		{[]string{"install", "missing", "--json"}, "invalid-package"},
		{[]string{"plan", "missing", "--json"}, "invalid-package"},
		{[]string{"install", "demo", "--harness", "codex", "--set", "unknown=v", "--json"}, "request-rejected"},
		{[]string{"trust", "missing", "--json"}, "request-rejected"},
		{[]string{"uninstall", "missing", "--json"}, "conflict"},
	} {
		var out, errOut bytes.Buffer
		if code := RunWithServiceInput(context.Background(), service, root, test.args, strings.NewReader("y\n"), &out, &errOut); code != ExitInvalid {
			t.Fatalf("%v exit = %d; stderr=%s", test.args, code, &errOut)
		}
		var diagnostics []Diagnostic
		if err := decodeReport(out.Bytes(), &diagnostics); err != nil || len(diagnostics) != 1 || diagnostics[0].Code != test.code {
			t.Fatalf("%v diagnostics = %+v, %v; want %s", test.args, diagnostics, err, test.code)
		}
		var envelope jsonEnvelope
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || string(envelope.Result) != "null" {
			t.Fatalf("%v result = %s, %v; want null", test.args, envelope.Result, err)
		}
	}
}
