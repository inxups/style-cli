package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	claudeadapter "github.com/inxups/style-cli/internal/adapter/claude"
	codexadapter "github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/speakeasy-api/agenthooks"
)

const runtimeTestSHA = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func TestClaudeNativeFailureMatchesToolOutputCondition(t *testing.T) {
	entry := runtimeTestEntry("pkg/failure", "tool.error", "event", "inject", staticBody("FAILURE_CONDITION_MATCHED"))
	entry.When.Conditions = []when.Condition{{Field: "tool.output", Op: "contains", Pattern: "Exit code 7"}}
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{"tool.error": {entry}}), store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, claudeadapter.NewEnricher(), agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile("../../agenthooks/agenthookstest/fixtures/claude/post_tool_use_failure.json")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runtime.Run(context.Background(), claudeRunArgs(), bytes.NewReader(payload), &stdout, &stderr); code != 0 {
		t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
	}
	want := `{"hookSpecificOutput":{"additionalContext":"FAILURE_CONDITION_MATCHED","hookEventName":"PostToolUseFailure"}}`
	if stdout.String() != want {
		t.Fatalf("failure output condition: got %s, want %s", stdout.String(), want)
	}
}

func TestRunDecodesClaudeHookAndEmitsOnlyWhitelistedBlock(t *testing.T) {
	root := t.TempDir()
	store, err := state.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reason := "blocked by STYLE"
	entry := runtimeTestEntry("pkg/guard", "tool.pre", "event", "block", compile.Body{Static: &reason})
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{"tool.pre": {entry}}), store)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	runtime, err := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	var stdout, stderr bytes.Buffer
	code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(claudeToolPrePayload), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
	}
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("Run output is not JSON: %q: %v", stdout.String(), err)
	}
	if len(output) != 1 {
		t.Fatalf("block returned unexpected output channels: %#v", output)
	}
	hookOutput, ok := output["hookSpecificOutput"].(map[string]any)
	if !ok || hookOutput["permissionDecision"] != "deny" || hookOutput["permissionDecisionReason"] != reason {
		t.Fatalf("provider block response = %#v", output)
	}
	if strings.Contains(stdout.String(), "rm -rf build") || strings.Contains(logs.String(), "rm -rf build") {
		t.Fatal("tool input leaked into response or logs")
	}
	stats, err := os.ReadFile(filepath.Join(root, "sess-claude-1", "main", "stats.jsonl"))
	if err != nil {
		t.Fatalf("read stats: %v", err)
	}
	if !bytes.Contains(stats, []byte(`"entry":"pkg/guard@tool.pre"`)) || bytes.Contains(stats, []byte("rm -rf build")) {
		t.Fatalf("stats should contain metadata only: %s", stats)
	}
	var record state.StatsRecord
	if err := json.Unmarshal(bytes.TrimSpace(stats), &record); err != nil {
		t.Fatal(err)
	}
	if record.Tokens == nil || *record.Tokens != tokens.EstimateText(reason).Tokens || record.BodySHA256 != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(reason))) || record.Point != "tool.pre" {
		t.Fatalf("cost metadata does not describe emitted body: %+v", record)
	}
}

func TestProcessProjectsSessionEffortFactsIntoPersistentState(t *testing.T) {
	root := t.TempDir()
	store, err := state.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := runtimeTestEntry("pkg/guard", "tool.pre", "event", "inject", staticBody("context"))
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{"tool.pre": {entry}}), store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	approx := false
	event := &agenthooks.Event{
		Provider:   agenthooks.ProviderClaudeCode,
		Kind:       agenthooks.KindToolPre,
		NativeName: "PreToolUse",
		Session: agenthooks.SessionInfo{
			ID: "session-facts", Model: "claude-sonnet", Effort: "high", EffortRaw: "HIGH",
			EffortApprox: &approx, HarnessVersion: "2.1.300",
		},
		Raw: []byte(`{"session_id":"session-facts","hook_event_name":"PreToolUse"}`),
	}
	if _, err := runtime.process(context.Background(), event, event); err != nil {
		t.Fatal(err)
	}
	session, _, err := store.Read("session-facts", "main")
	if err != nil {
		t.Fatal(err)
	}
	if session.Model != "claude-sonnet" || session.ThinkingEffort != "high" || session.EffortRaw != "HIGH" || session.EffortApprox == nil || *session.EffortApprox || session.HarnessVersion != "2.1.300" {
		t.Fatalf("persisted session facts = %#v", session)
	}
}

func TestRunMatchesCodexAndClaudeFixturesToExactBlockResponses(t *testing.T) {
	const reason = "blocked by STYLE"
	entry := runtimeTestEntry("pkg/guard", "tool.pre", "tool", "block", staticBody(reason))
	entry.Match = &compile.Match{Canonical: []string{"shell"}}
	index := runtimeTestIndex(map[string][]compile.Entry{"tool.pre": {entry}})
	fixtures := []struct {
		provider agenthooks.Provider
		path     string
		want     string
	}{
		{
			provider: agenthooks.ProviderClaudeCode,
			path:     "../../agenthooks/agenthookstest/fixtures/claude/pre_tool_use.json",
			want:     `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by STYLE"}}`,
		},
		{
			provider: agenthooks.ProviderCodex,
			path:     "../../agenthooks/agenthookstest/fixtures/codex/pre_tool_use.json",
			want:     `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by STYLE"}}`,
		},
	}
	for _, fixture := range fixtures {
		t.Run(string(fixture.provider), func(t *testing.T) {
			payload, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			store, err := state.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.New(index, store)
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := New(core, store, testEnricher{}, fixture.provider, false)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			args := []string{"agenthooks", "run", "--provider=" + string(fixture.provider)}
			if code := runtime.Run(context.Background(), args, bytes.NewReader(payload), &stdout, &stderr); code != 0 {
				t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
			}
			if stdout.String() != fixture.want {
				t.Fatalf("hook output = %q, want %q", stdout.String(), fixture.want)
			}
		})
	}
}

func TestCodexSubagentStartRuntimeDeliversContextForAgentType(t *testing.T) {
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry := runtimeTestEntry("pkg/subagent", "subagent.start", "event", "inject", staticBody("review the repository instructions"))
	entry.When.Agent = []string{"reviewer"}
	entry.When.ThinkingEffortApprox = nil
	index := runtimeTestIndex(map[string][]compile.Entry{"subagent.start": {entry}})
	core, err := engine.New(index, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, codexadapter.NewEnricher(), agenthooks.ProviderCodex, false)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	const payload = `{"session_id":"codex-subagent-session","cwd":"/repo","hook_event_name":"SubagentStart","agent_id":"child-7","agent_type":"reviewer"}`
	var stdout, stderr bytes.Buffer
	code := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=codex"}, strings.NewReader(payload), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
	}
	want := `{"hookSpecificOutput":{"additionalContext":"review the repository instructions","hookEventName":"SubagentStart"}}`
	if stdout.String() != want {
		t.Fatalf("SubagentStart response = %q, want %q", stdout.String(), want)
	}
	if _, _, err := store.Read("codex-subagent-session", "child-7"); err != nil {
		t.Fatalf("subagent scope state was not written: %v", err)
	}
}

func TestClaudePromptBlockSuppressesOriginalPrompt(t *testing.T) {
	const reason = "prompt needs a safer form"
	entry := runtimeTestEntry("pkg/prompt", "prompt.submitted", "event", "block", staticBody(reason))
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{"prompt.submitted": {entry}}), store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	payload := `{"session_id":"prompt-session","hook_event_name":"UserPromptSubmit","prompt":"sensitive original prompt"}`
	if code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(payload), &stdout, &stderr); code != 0 {
		t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
	}
	want := `{"decision":"block","hookSpecificOutput":{"hookEventName":"UserPromptSubmit","suppressOriginalPrompt":true},"reason":"prompt needs a safer form"}`
	if stdout.String() != want {
		t.Fatalf("prompt block response = %q, want %q", stdout.String(), want)
	}
}

func TestClaudePromptExpansionRuntimeUsesAdapterAndRunnerHandler(t *testing.T) {
	const reason = "skill disabled"
	entry := runtimeTestEntry("pkg/skill", "skill.explicit", "skill", "block", staticBody(reason))
	entry.Match = &compile.Match{Skill: []string{"review"}}
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{"skill.explicit": {entry}}), store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, claudeadapter.NewEnricher(), agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	payload := `{"session_id":"expansion-runtime","cwd":"/repo","hook_event_name":"UserPromptExpansion","expansion_type":"slash_command","command_name":"review","prompt":"/review","effort":{"level":"high"}}`
	if code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(payload), &stdout, &stderr); code != 0 {
		t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
	}
	want := `{"decision":"block","reason":"skill disabled"}`
	if stdout.String() != want {
		t.Fatalf("UserPromptExpansion response = %q, want %q", stdout.String(), want)
	}
}

func TestMissingOrInvalidIndexUsesCodecFailPolicy(t *testing.T) {
	for _, indexCase := range []struct {
		name string
		data []byte
	}{
		{name: "invalid index", data: []byte("{")},
		{name: "missing index"},
	} {
		for _, policy := range []struct {
			name       string
			failClosed bool
			wantDeny   bool
		}{
			{name: "fail-open"},
			{name: "fail-closed", failClosed: true, wantDeny: true},
		} {
			t.Run(indexCase.name+"/"+policy.name, func(t *testing.T) {
				root := t.TempDir()
				indexPath := filepath.Join(root, "index.json")
				if indexCase.data != nil {
					if err := os.WriteFile(indexPath, indexCase.data, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				runtime, err := Load(indexPath, filepath.Join(root, "state"), testEnricher{}, agenthooks.ProviderClaudeCode, policy.failClosed)
				if err != nil {
					t.Fatal(err)
				}
				var logs, stdout, stderr bytes.Buffer
				runtime.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
				code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(claudeToolPrePayload), &stdout, &stderr)
				wantCode := 0
				if policy.wantDeny {
					wantCode = 1
				}
				if code != wantCode {
					t.Fatalf("Run exit = %d, want %d; stderr=%q", code, wantCode, stderr.String())
				}
				if policy.wantDeny {
					if !strings.Contains(stdout.String(), `"permissionDecision":"deny"`) {
						t.Fatalf("fail-closed output = %q", stdout.String())
					}
				} else if stdout.String() != "{}" {
					t.Fatalf("fail-open output = %q, want {}", stdout.String())
				}
				if !strings.Contains(logs.String(), "index-invalid") {
					t.Fatalf("missing safe error code in logs: %s", logs.String())
				}
				if strings.Contains(logs.String(), "rm -rf build") || strings.Contains(logs.String(), indexPath) {
					t.Fatal("index details or tool input leaked into logs")
				}
			})
		}
	}
}

func TestStateFailureUsesFailOpenAndLogsOnlySafeCode(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, "state")
	badState := filepath.Join(stateRoot, "sess-claude-1", "session.json")
	if err := os.MkdirAll(filepath.Dir(badState), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badState, []byte(`{"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := state.New(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{}), store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	var logs, stdout, stderr bytes.Buffer
	runtime.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(claudeToolPrePayload), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || stdout.String() != "{}" {
		t.Fatalf("state failure response: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(logs.String(), `"code":"state-error"`) {
		t.Fatalf("state failure did not produce a safe error code: %s", logs.String())
	}
	if strings.Contains(logs.String(), "clean build") || strings.Contains(logs.String(), "unexpected") || strings.Contains(logs.String(), badState) {
		t.Fatalf("state details or tool input leaked into logs: %s", logs.String())
	}
}

func TestRuntimeReevaluatesDynamicSystemWhenModelChanges(t *testing.T) {
	root := t.TempDir()
	store, err := state.New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	body := compile.Body{Parts: []compile.Fragment{
		{Text: "model="}, {Variable: &compile.Variable{Name: "model"}},
		{Text: " effort="}, {Variable: &compile.Variable{Name: "thinking_effort"}},
	}}
	startup := runtimeTestEntry("pkg/system", "session.start", "system", "inject", body)
	startup.Sources = runtimeStringSlice("startup", "compact")
	startup.ReinjectOnCompact = true
	prompt := runtimeTestEntry("pkg/system", "prompt.submitted", "system", "inject", body)
	prompt.Reevaluate = true
	prompt.ReinjectOnCompact = true
	core, err := engine.New(runtimeTestIndex(map[string][]compile.Entry{
		"session.start": {startup}, "prompt.submitted": {prompt},
	}), store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		payload    string
		wantOutput string
	}{
		{name: "startup", payload: `{"session_id":"dynamic-session","source":"startup","hook_event_name":"SessionStart","model":"model-a","test_effort":"low"}`, wantOutput: "model=model-a effort=low"},
		{name: "same model and effort is deduplicated", payload: `{"session_id":"dynamic-session","prompt_id":"prompt-a","hook_event_name":"UserPromptSubmit","model":"model-a","test_effort":"low","prompt":"hello"}`},
		{name: "effort change is reevaluated", payload: `{"session_id":"dynamic-session","prompt_id":"prompt-b","hook_event_name":"UserPromptSubmit","model":"model-a","test_effort":"high","prompt":"hello"}`, wantOutput: "model=model-a effort=high"},
		{name: "switch event only updates model state", payload: `{"session_id":"dynamic-session","hook_event_name":"PostModelSwitch","to_model":"model-b"}`},
		{name: "changed model is reevaluated", payload: `{"session_id":"dynamic-session","prompt_id":"prompt-c","hook_event_name":"UserPromptSubmit","prompt":"hello"}`, wantOutput: "model=model-b effort=high"},
		{name: "compact advances epoch and reinjects opted-in rule", payload: `{"session_id":"dynamic-session","source":"compact","hook_event_name":"SessionStart"}`, wantOutput: "model=model-b effort=high"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(test.payload), &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run exit=%d stderr=%q", code, stderr.String())
			}
			if test.wantOutput == "" {
				if stdout.String() != "{}" {
					t.Fatalf("unchanged model output = %q, want {}", stdout.String())
				}
				return
			}
			if !strings.Contains(stdout.String(), test.wantOutput) {
				t.Fatalf("dynamic output %q does not contain %q", stdout.String(), test.wantOutput)
			}
		})
	}
}

type testEnricher struct{}

func (testEnricher) StateUpdate(event *agenthooks.Event, typed any, previous state.Session) state.EventUpdate {
	update := state.EventUpdate{Model: event.Session.Model}
	var testFields struct {
		Effort string `json:"test_effort"`
	}
	if json.Unmarshal(event.Raw, &testFields) == nil && testFields.Effort != "" {
		update.ThinkingEffort = testFields.Effort
	}
	if event.NativeName == "PostModelSwitch" {
		var switchEvent struct {
			ToModel string `json:"to_model"`
		}
		if json.Unmarshal(event.Raw, &switchEvent) == nil {
			update.Model = switchEvent.ToModel
		}
	}
	if start, ok := typed.(*agenthooks.SessionStartEvent); ok {
		update.Source = start.Source
	}
	return update
}

func (testEnricher) Enrich(event *agenthooks.Event, typed any, session state.Session, scope state.Scope) (engine.Input, tokens.InputLimit, error) {
	capabilities := engine.Capabilities{
		AddContext: event.Can(agenthooks.CapAddContext), SystemMessage: event.Can(agenthooks.CapSystemMessage),
		Deny: event.Can(agenthooks.CapDeny),
	}
	point := engine.Point{Name: string(event.Kind), Capabilities: capabilities}
	point.When = when.Input{Model: session.Model, ThinkingEffort: session.ThinkingEffort, ThinkingEffortApprox: boolPointer(false)}
	if event.Agent != nil {
		point.When.AgentID, point.When.AgentType = event.Agent.ID, event.Agent.Type
	}
	point.When.Harness = map[string]string{"name": string(event.Provider), "permission_mode": event.Session.PermissionMode}
	values := map[string]string{
		"model": session.Model, "harness.name": string(event.Provider),
		"harness.cwd": event.Session.CWD, "harness.permission_mode": event.Session.PermissionMode,
	}
	if tool, ok := typed.(*agenthooks.ToolPreEvent); ok {
		point.CanonicalTool, point.ToolName = string(tool.Tool.Canonical), tool.Tool.Name
		var input map[string]any
		if json.Unmarshal(tool.Tool.Input, &input) == nil {
			for key, value := range input {
				if text, ok := value.(string); ok {
					field := "tool.input." + key
					point.When.Fields = when.FieldMap{field: text}
					values[field] = text
				}
			}
		}
	}
	point.Values = values
	return engine.Input{
		Harness: string(event.Provider), Scope: scope,
		Variables: map[string]string{"model": session.Model, "harness.name": string(event.Provider)},
		Points:    []engine.Point{point},
	}, tokens.InputLimit{}, nil
}

func runtimeTestEntry(rule, point, target, action string, body compile.Body) compile.Entry {
	return compile.Entry{
		Rule: rule, Entry: rule + "@" + point, Source: "rules/test.md", SourceSHA256: runtimeTestSHA,
		BodySHA256: runtimeTestSHA, Order: compile.Order{Priority: 1, PackageSeq: 1}, Target: target, Action: action,
		When: when.Rule{ThinkingEffortApprox: boolPointer(false), Agent: []string{"main"}}, Repeat: "session",
		ReinjectOnCompact: false, Needs: []string{}, Body: body, Estimate: tokens.Estimate{}, DegradedFrom: nil,
	}
}

func runtimeTestIndex(events map[string][]compile.Entry) compile.Index {
	return compile.Index{
		Format: compile.Format, StyleVersion: "0.3.0", Packages: []compile.PackageInfo{},
		Static:    map[string][]compile.StaticEntry{"claude-code": {}, "codex": {}},
		Harnesses: map[string]map[string][]compile.Entry{"claude-code": events, "codex": events},
	}
}

func runtimeStringSlice(items ...string) *[]string { return &items }
func boolPointer(value bool) *bool                 { return &value }
func staticBody(value string) compile.Body         { return compile.Body{Static: &value} }

func claudeRunArgs() []string { return []string{"agenthooks", "run", "--provider=claude-code"} }

const claudeToolPrePayload = `{"session_id":"sess-claude-1","transcript_path":"/tmp/claude/transcript.jsonl","cwd":"/work/repo","hook_event_name":"PreToolUse","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"rm -rf build/","description":"clean build"},"tool_use_id":"toolu_01ABC"}`
