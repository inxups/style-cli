package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
)

const a1Hash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func a1Entry(rule, point, body string) compile.Entry {
	return compile.Entry{Rule: "fixture/" + rule, Entry: "fixture/" + rule + "@" + point, Source: "rules/test.md", SourceSHA256: a1Hash, BodySHA256: a1Hash, Order: compile.Order{Priority: 50, PackageSeq: 1}, Target: "event", Action: "inject", When: when.Rule{Agent: []string{"main"}, Event: point}, Repeat: "session", Needs: []string{}, Body: compile.Body{Static: &body}, Estimate: tokens.Estimate{}}
}
func a1Fixture(t *testing.T, harness, event string, events map[string][]compile.Entry) (*Service, ExplainRequest) {
	t.Helper()
	root := t.TempDir()
	index := compile.Index{Format: 1, StyleVersion: "0.3.0", Packages: []compile.PackageInfo{}, Static: map[string][]compile.StaticEntry{harness: {}}, Harnesses: map[string]map[string][]compile.Entry{harness: events}}
	data, err := compile.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "index.json")
	writeSyncFile(t, indexPath, string(data))
	payloadPath := filepath.Join(root, "payload.json")
	writeSyncFile(t, payloadPath, fmt.Sprintf(`{"session_id":"s","hook_event_name":%q,"tool_name":"Bash","tool_input":{"command":"CANARY_PROMPT"},"prompt":"hello $one $two","model":"model-a","source":"startup"}`, event))
	return testService(t), ExplainRequest{Harness: harness, Event: event, Index: indexPath, Payload: payloadPath}
}
func a1Facts(t *testing.T, request *ExplainRequest, fields map[string]any) {
	t.Helper()
	data := readSyncFile(t, request.Payload)
	fields["format"] = 1
	fields["harness"] = request.Harness
	fields["payload_sha256"] = explainSHA(data)
	encoded, _ := json.Marshal(fields)
	request.Facts = filepath.Join(filepath.Dir(request.Payload), "facts.json")
	writeSyncFile(t, request.Facts, string(encoded))
}
func a1Code(result ExplainResult) string {
	if len(result.Diagnostics) == 0 {
		return ""
	}
	return result.Diagnostics[len(result.Diagnostics)-1].Code
}

func TestM3A1CLIContractAndDeterministicReports(t *testing.T) {
	service, request := a1Fixture(t, "codex", "PreToolUse", map[string][]compile.Entry{"tool.pre": {a1Entry("one", "tool.pre", "Visible rule.\x1b[31m")}})
	root := filepath.Dir(request.Payload)
	before := snapshotStatsTree(t, root)
	args := []string{"explain", "--harness", request.Harness, "--event", request.Event, "--payload", request.Payload, "--index", request.Index, "--json"}
	var first, second, stderr bytes.Buffer
	if exit := RunWithService(context.Background(), service, root, args, &first, &stderr); exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%s json=%s", exit, stderr.String(), first.String())
	}
	if exit := RunWithService(context.Background(), service, root, args, &second, &stderr); exit != 0 || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("report not deterministic")
	}
	var result ExplainResult
	if json.Unmarshal(first.Bytes(), &result) != nil || result.Status != "ok" || len(result.Entries) != 1 || result.Entries[0].Output != "output" || !strings.Contains(result.Wire.Stdout, "Visible rule.") {
		t.Fatalf("report %s", first.String())
	}
	if bytes.Contains(first.Bytes(), []byte("CANARY_PROMPT")) || bytes.Contains(first.Bytes(), []byte("hello $one")) {
		t.Fatal("report copied raw payload")
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, root)) {
		t.Fatal("explain mutated inputs")
	}
	var text bytes.Buffer
	if RunWithService(context.Background(), service, root, args[:len(args)-1], &text, &stderr) != 0 || strings.Contains(text.String(), "\x1b") {
		t.Fatalf("unsafe text report %s", text.String())
	}
	var inferred bytes.Buffer
	if exit := RunWithService(context.Background(), service, root, []string{"explain", "--harness", request.Harness, "--payload", request.Payload, "--index", request.Index, "--json"}, &inferred, &stderr); exit != 0 || !bytes.Equal(first.Bytes(), inferred.Bytes()) {
		t.Fatalf("event not inferred from payload: exit=%d %s", exit, inferred.String())
	}
	bogus := filepath.Join(t.TempDir(), "bogus.json")
	writeSyncFile(t, bogus, `{"session_id":"s","hook_event_name":"Bogus"}`)
	var unsupported bytes.Buffer
	if exit := RunWithService(context.Background(), service, root, []string{"explain", "--harness", request.Harness, "--payload", bogus, "--index", request.Index, "--json"}, &unsupported, &stderr); exit != 1 || !strings.Contains(unsupported.String(), `"event-unsupported"`) || !strings.Contains(unsupported.String(), `"event": "Bogus"`) {
		t.Fatalf("inferred unsupported event: exit=%d %s", exit, unsupported.String())
	}
	var mismatch bytes.Buffer
	if exit := RunWithService(context.Background(), service, root, []string{"explain", "--harness", request.Harness, "--event", "SessionStart", "--payload", request.Payload, "--index", request.Index, "--json"}, &mismatch, &stderr); exit != 1 || !strings.Contains(mismatch.String(), `"event-mismatch"`) {
		t.Fatalf("explicit event mismatch: exit=%d %s", exit, mismatch.String())
	}
	for _, bad := range [][]string{
		{"--json"}, {"--harness", "codex", "--event", "PreToolUse"}, {"--harness", "auto", "--event", "PreToolUse", "--payload", "x"}, {"--harness", "codex,claude-code", "--event", "PreToolUse", "--payload", "x"},
		{"--harness", "codex", "--harness", "codex", "--event", "PreToolUse", "--payload", "x", "--json"}, {"--bogus", "x", "--json"}, {"--harness", "codex", "--event", "PreToolUse", "--payload", "--json"},
	} {
		var out, errOut bytes.Buffer
		if RunWithService(context.Background(), service, root, append([]string{"explain"}, bad...), &out, &errOut) != ExitUsage || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("usage accepted %v: %s %s", bad, out.String(), errOut.String())
		}
	}
}

func TestM3A1AllNativeEventsAndKnownNoHandlers(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		events := []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PermissionRequest", "Stop", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact"}
		if harness == "claude-code" {
			events = append(events, "UserPromptExpansion", "PostToolUseFailure", "Notification", "FileChanged", "MessageDisplay", "PostModelSwitch")
		}
		for _, event := range events {
			t.Run(harness+"/"+event, func(t *testing.T) {
				service, request := a1Fixture(t, harness, event, map[string][]compile.Entry{})
				result, err := service.Explain(context.Background(), request)
				if err != nil || result.Status != "ok" || result.Wire == nil {
					t.Fatalf("%+v %v", result, err)
				}
				if event == "PreToolUse" && harness == "codex" && result.Wire.Stdout != "" {
					t.Fatal("Codex no-op must stay empty")
				}
				if event == "SessionStart" || event == "UserPromptSubmit" || event == "PreToolUse" || event == "PostToolUse" || event == "PermissionRequest" || event == "SubagentStart" || event == "PostToolUseFailure" {
					if len(result.Points) == 0 {
						t.Fatalf("missing evaluation points %+v", result)
					}
				}
			})
		}
	}
	service, request := a1Fixture(t, "claude-code", "PostModelSwitch", map[string][]compile.Entry{})
	request.Harness = "claude"
	if result, err := service.Explain(context.Background(), request); err != nil || result.Harness != "claude-code" {
		t.Fatalf("alias %+v %v", result, err)
	}
}

func TestM3A1InputFailuresAndNativeFailPolicy(t *testing.T) {
	for _, test := range []struct {
		name, want string
		change     func(*testing.T, *ExplainRequest)
	}{
		{"unknown-event", "event-unsupported", func(t *testing.T, r *ExplainRequest) { r.Event = "Invented" }},
		{"unknown-harness", "harness-unknown", func(t *testing.T, r *ExplainRequest) { r.Harness = "unknown" }},
		{"mismatch", "event-mismatch", func(t *testing.T, r *ExplainRequest) { r.Event = "PostToolUse" }},
		{"invalid-json", "payload-invalid", func(t *testing.T, r *ExplainRequest) { writeSyncFile(t, r.Payload, "{") }},
		{"tail", "payload-invalid", func(t *testing.T, r *ExplainRequest) {
			writeSyncFile(t, r.Payload, string(readSyncFile(t, r.Payload))+"{}")
		}},
		{"array", "payload-invalid", func(t *testing.T, r *ExplainRequest) { writeSyncFile(t, r.Payload, "[]") }},
		{"utf8", "payload-invalid", func(t *testing.T, r *ExplainRequest) { writeSyncFile(t, r.Payload, string([]byte{0xff})) }},
		{"session", "payload-invalid", func(t *testing.T, r *ExplainRequest) { writeSyncFile(t, r.Payload, `{"hook_event_name":"PreToolUse"}`) }},
		{"bad-index", "index-invalid", func(t *testing.T, r *ExplainRequest) { writeSyncFile(t, r.Index, `{"format":2}`) }},
		{"linked-input", "payload-invalid", func(t *testing.T, r *ExplainRequest) {
			path := r.Payload + ".link"
			if err := os.Symlink(r.Payload, path); err != nil {
				t.Fatal(err)
			}
			r.Payload = path
		}},
		{"directory-input", "payload-invalid", func(t *testing.T, r *ExplainRequest) { r.Payload = filepath.Dir(r.Payload) }},
		{"facts-schema", "facts-invalid", func(t *testing.T, r *ExplainRequest) { a1Facts(t, r, map[string]any{"secret_key": "CANARY"}) }},
		{"facts-conflict", "facts-conflict", func(t *testing.T, r *ExplainRequest) {
			a1Facts(t, r, map[string]any{})
			writeSyncFile(t, r.Payload, string(readSyncFile(t, r.Payload))+" ")
		}},
		{"foreign-facts", "facts-conflict", func(t *testing.T, r *ExplainRequest) { a1Facts(t, r, map[string]any{"codex_config_effort": "high"}) }},
		{"skill-wrong-event", "facts-conflict", func(t *testing.T, r *ExplainRequest) { a1Facts(t, r, map[string]any{"claude_skill_content": "CANARY"}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, request := a1Fixture(t, "claude-code", "PreToolUse", map[string][]compile.Entry{})
			test.change(t, &request)
			result, err := service.Explain(context.Background(), request)
			if err == nil || result.Status != "error" || a1Code(result) != test.want {
				t.Fatalf("got %+v %v", result, err)
			}
		})
	}
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprintf("policy/%t", closed), func(t *testing.T) {
			entry := a1Entry("bad-block", "tool.pre", "")
			entry.Action = "block"
			service, request := a1Fixture(t, "claude-code", "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
			request.FailClosed = closed
			result, err := service.Explain(context.Background(), request)
			if err == nil || result.Wire == nil || a1Code(result) != "dispatch-error" {
				t.Fatalf("management swallowed failure %+v %v", result, err)
			}
			want := 0
			if closed {
				want = 1
			}
			if result.Wire.ExitCode != want {
				t.Fatalf("wire=%+v", result.Wire)
			}
		})
	}
	service, request := a1Fixture(t, "claude-code", "PreToolUse", map[string][]compile.Entry{})
	writeSyncFile(t, request.Payload, `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":42}`)
	result, err := service.Explain(context.Background(), request)
	if err == nil || a1Code(result) != "decode-error" || result.Wire != nil {
		t.Fatalf("decode fail-open confused with success %+v %v", result, err)
	}
}

func TestM3A1SnapshotReadOnlyLifecycleAndRepeat(t *testing.T) {
	service, request := a1Fixture(t, "codex", "PreToolUse", map[string][]compile.Entry{"tool.pre": {a1Entry("once", "tool.pre", "Once.")}})
	source := filepath.Join(t.TempDir(), "state")
	store, _ := state.New(source)
	_, scope, err := store.Update("s", "main", state.EventUpdate{PromptSubmitted: true})
	if err != nil {
		t.Fatal(err)
	}
	_, allowed, err := store.Claim(state.ClaimInput{Scope: scope, Repeat: "session", Action: "inject", Event: "tool.pre", Rule: "fixture/once", Body: "Once."})
	if err != nil || !allowed {
		t.Fatal(err)
	}
	_, _, _ = store.Update("other", "other", state.EventUpdate{Source: "clear"})
	request.StateDir = source
	before := snapshotStatsTree(t, source)
	result, err := service.Explain(context.Background(), request)
	if err != nil || !result.State.SeedScopeExists || result.State.Before.PromptCount != 1 || len(result.Entries) != 1 || result.Entries[0].Repeat != "repeat-seen" {
		t.Fatalf("snapshot %+v %v", result, err)
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, source)) {
		t.Fatalf("source state metadata changed: before=%+v after=%+v", before, snapshotStatsTree(t, source))
	}
	request.StateDir = ""
	result, err = service.Explain(context.Background(), request)
	if err != nil || result.Entries[0].Output != "output" || result.State.Before.PromptCount != 0 {
		t.Fatalf("empty mode %+v %v", result, err)
	}
	request.StateDir = source
	request.Event = "SessionStart"
	writeSyncFile(t, request.Payload, `{"session_id":"s","hook_event_name":"SessionStart","source":"clear"}`)
	result, err = service.Explain(context.Background(), request)
	if err != nil || result.State.After.Generation != result.State.Before.Generation+1 {
		t.Fatalf("clear %+v %v", result, err)
	}
	writeSyncFile(t, request.Payload, `{"session_id":"s","hook_event_name":"SessionStart","source":"compact"}`)
	result, err = service.Explain(context.Background(), request)
	if err != nil || result.State.After.Epoch != 1 {
		t.Fatalf("compact %+v %v", result, err)
	}
	if !reflect.DeepEqual(before, snapshotStatsTree(t, source)) {
		t.Fatal("lifecycle changed source")
	}
}

func TestM3A1FactsResolutionSkillMCPAndConcurrency(t *testing.T) {
	t.Setenv("CLAUDE_EFFORT", "high")
	config := t.TempDir()
	writeSyncFile(t, filepath.Join(config, "config.toml"), "model_reasoning_effort='high'")
	t.Setenv("CODEX_HOME", config)
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			entry := a1Entry("effort", "tool.pre", "Effort match.")
			entry.When.ThinkingEffort = []string{"high"}
			service, request := a1Fixture(t, harness, "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
			without, err := service.Explain(context.Background(), request)
			if err != nil || without.Entries[0].When != "missing-data" {
				t.Fatalf("external env leaked %+v %v", without, err)
			}
			field := "codex_config_effort"
			if harness == "claude-code" {
				field = "claude_env_effort"
			}
			a1Facts(t, &request, map[string]any{field: "high"})
			with, err := service.Explain(context.Background(), request)
			if err != nil || with.Entries[0].Output != "output" {
				t.Fatalf("provided effort %+v %v", with, err)
			}
			results := make(chan string, 8)
			var group sync.WaitGroup
			for i := 0; i < 8; i++ {
				group.Add(1)
				go func() {
					defer group.Done()
					result, err := service.Explain(context.Background(), request)
					if err != nil {
						results <- err.Error()
						return
					}
					data, _ := json.Marshal(result)
					results <- string(data)
				}()
			}
			group.Wait()
			close(results)
			encoded, _ := json.Marshal(with)
			for data := range results {
				if data != string(encoded) {
					t.Fatal("concurrent replay isolation mismatch")
				}
			}
		})
	}
	entry := a1Entry("skill", "tool.post", "Matched captured Skill.")
	entry.When.Conditions = []when.Condition{{Field: "tool.output", Op: "contains", Pattern: "CAPTURED"}}
	service, request := a1Fixture(t, "claude-code", "PostToolUse", map[string][]compile.Entry{"tool.post": {entry}})
	writeSyncFile(t, request.Payload, `{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Skill","tool_input":{"skill":"private"},"tool_response":"native"}`)
	a1Facts(t, &request, map[string]any{"claude_skill_content": "CAPTURED"})
	result, err := service.Explain(context.Background(), request)
	if err != nil || result.Entries[0].Output != "output" {
		t.Fatalf("skill %+v %v", result, err)
	}
	writeSyncFile(t, request.Payload, `{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"mcp__server__read"}`)
	a1Facts(t, &request, map[string]any{"mcp": map[string]string{"server": "wrong", "tool": "read"}})
	result, err = service.Explain(context.Background(), request)
	if err == nil || a1Code(result) != "facts-conflict" {
		t.Fatalf("MCP conflict %+v %v", result, err)
	}
}

func TestM3A1CancellationCleanupAndUnavailableRuntime(t *testing.T) {
	service, request := a1Fixture(t, "codex", "PreToolUse", map[string][]compile.Entry{})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := service.Explain(cancelled, request); err == nil || a1Code(result) != "cancelled" {
		t.Fatalf("cancel %+v %v", result, err)
	}
	ops := defaultExplainOps()
	created := ""
	ops.mktemp = func(base, pattern string) (string, error) {
		dir, err := os.MkdirTemp(t.TempDir(), pattern)
		created = dir
		return dir, err
	}
	ops.remove = func(string) error { return errors.New("SECRET cleanup") }
	result, err := service.explain(context.Background(), request, ops)
	if err == nil || a1Code(result) != "cleanup-failed" || !strings.Contains(result.Diagnostics[len(result.Diagnostics)-1].Message, created) {
		t.Fatalf("cleanup %+v %v", result, err)
	}
	_ = os.RemoveAll(created)
	ops = defaultExplainOps()
	ops.mktemp = func(string, string) (string, error) { return "", errors.New("SECRET disk") }
	if result, err := service.explain(context.Background(), request, ops); err == nil || a1Code(result) != "temporary-state-error" {
		t.Fatalf("mktemp %+v %v", result, err)
	}
	service.definitions[adapter.ProviderID("fixture")] = service.definitions["codex"]
	request.Harness = "fixture"
	if result, err := service.Explain(context.Background(), request); err == nil || a1Code(result) != "runtime-unavailable" {
		t.Fatalf("runtime %+v %v", result, err)
	}
}

func TestM3A1FixedGoldenAndOrdinaryRuntimeWire(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "explain")
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			service := testService(t)
			request := ExplainRequest{Harness: harness, Event: "PreToolUse", Payload: filepath.Join(root, "pre-tool.json"), Index: filepath.Join(root, "index.json")}
			result, err := service.Explain(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			var encoded bytes.Buffer
			encoder := json.NewEncoder(&encoded)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(result); err != nil {
				t.Fatal(err)
			}
			expected := readSyncFile(t, filepath.Join(root, harness+".golden.json"))
			if !bytes.Equal(expected, encoded.Bytes()) {
				t.Fatalf("golden mismatch\n%s", encoded.String())
			}
			// Independently exercise the ordinary Runtime.Run path and its real codecs.
			store, _ := state.New(t.TempDir())
			core, err := engine.LoadIndexBytes(readSyncFile(t, request.Index), store)
			if err != nil {
				t.Fatal(err)
			}
			var enricher adapter.RuntimeEnricher
			if harness == "codex" {
				e := codex.NewEnricher()
				e.Home = t.TempDir()
				e.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) { return agenthooks.CodexLaunchSettings{}, false }
				e.ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
				enricher = e
			} else {
				e := claude.NewEnricher()
				e.Environment = func() string { return "" }
				enricher = e
			}
			runtime, err := styleruntime.New(core, store, enricher, agenthooks.Provider(harness), false)
			if err != nil {
				t.Fatal(err)
			}
			runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			runtime.CodecStateDir = t.TempDir()
			var stdout, stderr bytes.Buffer
			exit := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + harness}, bytes.NewReader(readSyncFile(t, request.Payload)), &stdout, &stderr, agenthooks.WithoutMCPResolution())
			if result.Wire.Stdout != stdout.String() || result.Wire.Stderr != stderr.String() || result.Wire.ExitCode != exit {
				t.Fatalf("ordinary wire differs: %+v %q %q %d", result.Wire, stdout.String(), stderr.String(), exit)
			}
		})
	}
}

func TestM3A1InputSizeLimitsAndJSONErrorReports(t *testing.T) {
	for _, field := range []string{"payload", "index", "facts"} {
		t.Run(field, func(t *testing.T) {
			service, request := a1Fixture(t, "codex", "PreToolUse", map[string][]compile.Entry{})
			path := request.Payload
			limit := int64(32 << 20)
			if field == "index" {
				path = request.Index
				limit = 16 << 20
			}
			if field == "facts" {
				path = filepath.Join(filepath.Dir(request.Payload), "facts.json")
				request.Facts = path
				limit = 1 << 20
			}
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Truncate(limit + 1); err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
			result, err := service.Explain(context.Background(), request)
			if err == nil || a1Code(result) != field+"-invalid" {
				t.Fatalf("size %+v %v", result, err)
			}
			args := []string{"explain", "--harness", request.Harness, "--event", request.Event, "--payload", request.Payload, "--index", request.Index, "--json"}
			if request.Facts != "" {
				args = append(args, "--facts", request.Facts)
			}
			var out, errOut bytes.Buffer
			if RunWithService(context.Background(), service, filepath.Dir(path), args, &out, &errOut) != 1 || json.Unmarshal(out.Bytes(), &result) != nil || result.Status != "error" {
				t.Fatalf("JSON error contract %s %s", out.String(), errOut.String())
			}
		})
	}
}

func TestM3A1ReplayCleansSuccessAndChangedSnapshot(t *testing.T) {
	service, request := a1Fixture(t, "codex", "PreToolUse", map[string][]compile.Entry{})
	for _, mode := range []string{"success", "changed", "cancel-after-copy"} {
		t.Run(mode, func(t *testing.T) {
			ops := defaultExplainOps()
			private := ""
			ops.mktemp = func(base, pattern string) (string, error) {
				dir, err := os.MkdirTemp(t.TempDir(), pattern)
				private = dir
				return dir, err
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode != "success" {
				request.StateDir = t.TempDir()
				ops.snapshot = func(context.Context, string, string, string, string) (state.Snapshot, error) {
					if mode == "changed" {
						return state.Snapshot{}, saferead.ErrChanged
					}
					cancel()
					return state.Snapshot{}, context.Canceled
				}
			}
			result, err := service.explain(ctx, request, ops)
			if mode == "success" && err != nil || mode != "success" && err == nil {
				t.Fatalf("result %+v %v", result, err)
			}
			if mode == "changed" && a1Code(result) != "state-snapshot-changed" {
				t.Fatal("snapshot race not distinct")
			}
			if _, err := os.Stat(private); !os.IsNotExist(err) {
				t.Fatal("private replay root was retained")
			}
		})
	}
}
