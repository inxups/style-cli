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
		{[]string{"trust", "missing", "--json"}, ExitInvalid, "error", "command-failed"},
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
			if envelope.Format != envelopeFormat || envelope.Command != test.args[0] || envelope.Status != test.status || envelope.Scope["kind"] != "user" || envelope.Scope["home"] != home {
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
}
