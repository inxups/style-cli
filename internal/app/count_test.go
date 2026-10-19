package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/tokencount"
)

func a15Options() tokencount.Options {
	return tokencount.Options{Mode: "exact", Provider: "openai", Encoding: "o200k_base"}
}

type a15Transport func(*http.Request) (*http.Response, error)

func (f a15Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestM3A15CountCLIReadOnlyAndInvalidInput(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	path := filepath.Join(root, "text ' 中文.txt")
	writeSyncFile(t, path, "hello world")
	s.counter = &tokencount.Counter{Key: func() string { t.Fatal("local counter read key"); return "" }, Client: &http.Client{Transport: a15Transport(func(*http.Request) (*http.Response, error) { t.Fatal("local counter used network"); return nil, nil })}}
	before := snapshotStatsTree(t, root)
	for _, args := range [][]string{{"count", path, "--json"}, {"count", path, "--exact=openai:o200k_base", "--json"}, {"count", path, "--exact", "openai:cl100k_base"}} {
		var out, logs bytes.Buffer
		code := RunWithService(context.Background(), s, root, args, &out, &logs)
		if code != 0 || logs.Len() != 0 || strings.Contains(out.String(), "hello world") {
			t.Fatal(code, out.String(), logs.String())
		}
		if strings.Contains(strings.Join(args, " "), "--json") {
			var r struct{ Count tokencount.Result }
			if json.Unmarshal(out.Bytes(), &r) != nil || r.Count.Tokens <= 0 || r.Count.TextSHA256 != explainSHA([]byte("hello world")) {
				t.Fatal(out.String())
			}
		}
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, root)) {
		t.Fatal("count modified input tree")
	}
	for _, bad := range [][]string{{"count"}, {"count", path, "--exact=openai"}, {"count", path, "--exact=openai:"}, {"count", path, "--exact=openai:auto"}, {"count", path, "--exact=bogus:x"}, {"count", path, "--exact=anthropic:bad model"}, {"count", path, "--exact=openai:o200k_base", "--exact=openai:o200k_base"}, {"count", path, "--exact"}, {"count", path, "--count=exact"}, {"count", path, "--count-provider=openai"}, {"count", path, path}} {
		var out, logs bytes.Buffer
		if code := RunWithService(context.Background(), s, root, bad, &out, &logs); code != ExitUsage || out.Len() != 0 {
			t.Fatal(bad, code, out.String())
		}
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{{"invalid", []byte{255}}, {"oversize", bytes.Repeat([]byte("a"), tokencount.MaxTextBytes+1)}, {"pathological-run", bytes.Repeat([]byte("a"), 4097)}} {
		t.Run(tc.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "input")
			if err := os.WriteFile(f, tc.body, 0600); err != nil {
				t.Fatal(err)
			}
			var out, logs bytes.Buffer
			if code := RunWithService(context.Background(), s, root, []string{"count", f, "--exact=openai:o200k_base", "--json"}, &out, &logs); code != 1 || out.Len() != 0 {
				t.Fatal(code, out.String())
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(path, link); err != nil {
			t.Skip("platform cannot create test symlink:", err)
		}
		var out, logs bytes.Buffer
		if code := RunWithService(context.Background(), s, root, []string{"count", link}, &out, &logs); code != 1 {
			t.Fatal(code)
		}
	})
}

const a15Manifest = "spec: style/v0.3\nname: counted\nversion: 1.0.0\ndescription: exact text fixture\nsettings: [{name: tone, default: direct}, {name: voice, default: plain}]\nrules:\n  - {id: static, target: system, text: '{{settings.tone}} {{settings.voice}}'}\n  - {id: warning, target: tool, tool: shell, at: before, action: warn, text: 'PRIVATE USER TEXT', model_text: hello world}\n  - {id: one, target: tool, tool: shell, at: before, text: hello world}\n  - {id: two, target: tool, tool: shell, at: before, text: hello world}\n  - {id: prompt, target: event, event: prompt.submitted, action: block, text: user only reason}\n"

func TestM3A15PlanCountsCompiledTextAndPreservesEstimate(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	t.Setenv("STYLE_HOME", filepath.Join(root, "home"))
	source := filepath.Join(root, "source")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a15Manifest)
	req := PlanRequest{Source: source, Harnesses: []string{"codex"}}
	baseline, err := s.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotStatsTree(t, root)
	req.Count = a15Options()
	actual, err := s.Plan(context.Background(), req)
	if err != nil || len(actual.Reports) != 1 {
		t.Fatal(actual, err)
	}
	counts := actual.Reports[0].Counts
	if len(counts) != 5 {
		t.Fatal(counts, actual.Diagnostics)
	}
	expected := map[string]string{"counted/static": "direct plain", "counted/warning": "hello world", "counted/one": "hello world", "counted/two": "hello world", "counted/prompt": ""}
	for _, item := range counts {
		want, ok := expected[item.Rule]
		if !ok || item.Count.TextSHA256 != explainSHA([]byte(want)) {
			t.Fatal(item)
		}
		if item.Rule == "counted/warning" && item.Count.Tokens != 2 {
			t.Fatal("warn used user text", item)
		}
		if item.Rule == "counted/prompt" && (item.Count.Tokens != 0 || item.Count.Scope != "not-model-input") {
			t.Fatal(item)
		}
	}
	actual.Reports[0].Counts = nil
	actual.Reports[0].CountScope = ""
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(baseline)
	if !bytes.Equal(a, b) {
		t.Fatal("exact changed planning/estimate/budget semantics")
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, root)) {
		t.Fatal("exact plan wrote input tree")
	}
	var out, logs bytes.Buffer
	code := RunWithService(context.Background(), s, root, []string{"plan", source, "--harness=codex", "--exact=openai:o200k_base", "--json"}, &out, &logs)
	if code != 0 || !strings.Contains(out.String(), `"counts"`) || strings.Contains(out.String(), "PRIVATE USER TEXT") {
		t.Fatal(code, out.String(), logs.String())
	}
}
func TestM3A15PlanDynamicRejectedBeforeNetwork(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	t.Setenv("STYLE_HOME", filepath.Join(root, "home"))
	source := filepath.Join(root, "source")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), "spec: style/v0.3\nname: counted\nversion: 1.0.0\ndescription: dynamic fixture\nrules:\n  - {id: static, target: system, text: STATIC}\n  - {id: dynamic, target: event, event: tool.pre, text: '{{model | default: \"none\"}}'}\n")
	s.counter = &tokencount.Counter{Key: func() string { t.Fatal("dynamic plan accessed key"); return "" }, Client: &http.Client{Transport: a15Transport(func(*http.Request) (*http.Response, error) { t.Fatal("dynamic plan used network"); return nil, nil })}}
	before := snapshotStatsTree(t, root)
	req := PlanRequest{Source: source, Harnesses: []string{"codex"}, Count: tokencount.Options{Mode: "exact", Provider: "anthropic", Model: "claude-fixture"}}
	_, err := s.Plan(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "use explain") {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, root)) {
		t.Fatalf("failed plan changed files: before=%+v after=%+v", before, snapshotStatsTree(t, root))
	}
}

func TestM3A15ExplainCountsFinalModelTextWithoutChangingWire(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, mode := range []string{"merged", "dynamic", "warn", "cap-drop", "tool-block", "prompt-block", "no-match", "repeat-seen"} {
			t.Run(harness+"/"+mode, func(t *testing.T) {
				event, point := "PreToolUse", "tool.pre"
				if mode == "prompt-block" {
					event, point = "UserPromptSubmit", "prompt.submitted"
				}
				entry := a1Entry("one", point, "hello world")
				entries := []compile.Entry{entry}
				want := "hello world"
				switch mode {
				case "merged":
					other := a1Entry("two", point, "再见。")
					entries = append(entries, other)
					want += "\n\n再见。"
				case "dynamic":
					missing := "none"
					entries[0].Body = compile.Body{Parts: []compile.Fragment{{Text: "Model="}, {Variable: &compile.Variable{Name: "model", Default: &missing}}}}
					want = "Model=model-a"
				case "warn":
					entries[0].Action = "warn"
					user := "PRIVATE-user-warning"
					entries[0].Body = compile.Body{Static: &user}
					body := entry.Body
					entries[0].ModelBody = &body
				case "cap-drop":
					huge := strings.Repeat("sentence ", 5000)
					entries[0].Body = compile.Body{Static: &huge}
					want = ""
				case "tool-block", "prompt-block":
					entries[0].Action = "block"
					if mode == "prompt-block" {
						want = ""
					}
				case "no-match":
					entries[0].When = when.Rule{Agent: []string{"main"}, Model: []string{"never-matches"}, Event: point}
					want = ""
				}
				s, req := a1Fixture(t, harness, event, map[string][]compile.Entry{point: entries})
				if mode == "repeat-seen" {
					req.StateDir = filepath.Join(t.TempDir(), "state")
					store, err := state.New(req.StateDir)
					if err != nil {
						t.Fatal(err)
					}
					_, scope, err := store.Update("s", "main", state.EventUpdate{})
					if err != nil {
						t.Fatal(err)
					}
					_, allowed, err := store.Claim(state.ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: point, Rule: "fixture/one", Body: want})
					if err != nil || !allowed {
						t.Fatal(allowed, err)
					}
					want = ""
				}
				baseline, err := s.Explain(context.Background(), req)
				if err != nil {
					t.Fatal(baseline, err)
				}
				req.Count = a15Options()
				before := snapshotStatsTree(t, filepath.Dir(req.Index))
				actual, err := s.Explain(context.Background(), req)
				if err != nil || actual.Count == nil || actual.Count.TextSHA256 != explainSHA([]byte(want)) || !reflect.DeepEqual(actual.Wire, baseline.Wire) {
					t.Fatal(actual.Count, actual.Wire, want, err)
				}
				if !reflect.DeepEqual(before, snapshotStatsTree(t, filepath.Dir(req.Index))) {
					t.Fatal("exact explain changed inputs")
				}
				if want == "" && (actual.Count.Tokens != 0 || actual.Count.Scope != "not-model-input") {
					t.Fatal(actual.Count)
				}
				actual.Count = nil
				a, _ := json.Marshal(actual)
				b, _ := json.Marshal(baseline)
				if !bytes.Equal(a, b) {
					t.Fatal("count changed normal diagnostic report")
				}
			})
		}
	}
}

func TestM3A15AnthropicCommandBoundaryAndFailureRecovery(t *testing.T) {
	s, req := a1Fixture(t, "claude-code", "PreToolUse", map[string][]compile.Entry{"tool.pre": {a1Entry("one", "tool.pre", "hello world")}})
	calls := 0
	bad := true
	s.counter = &tokencount.Counter{Key: func() string { return "fixture-key" }, Client: &http.Client{Transport: a15Transport(func(request *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(data), "hello world") {
			t.Fatal("counted wrong text")
		}
		if bad {
			return nil, fmt.Errorf("fixture-key PRIVATE")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"input_tokens":11}`)), Request: request}, nil
	})}}
	req.Count = tokencount.Options{Mode: "exact", Provider: "anthropic", Model: "claude-fixture"}
	before := snapshotStatsTree(t, filepath.Dir(req.Index))
	result, err := s.Explain(context.Background(), req)
	if err == nil || result.Status != "error" || a1Code(result) != "count-failed" || result.Count != nil || result.Wire == nil {
		t.Fatal(result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "fixture-key") || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("unsafe API error")
	}
	bad = false
	result, err = s.Explain(context.Background(), req)
	if err != nil || calls != 2 || result.Count == nil || result.Count.Tokens != 11 || result.Count.Scope != "anthropic-messages-request" {
		t.Fatal(result.Count, calls, err)
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, filepath.Dir(req.Index))) {
		t.Fatal("count failure or recovery changed inputs")
	}
	// Ordinary replay cannot use configured counting transport without exact.
	req.Count = tokencount.Options{}
	if _, err = s.Explain(context.Background(), req); err != nil || calls != 2 {
		t.Fatal(calls, err)
	}
}
