package agenthooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM3A1OfflineCodecSupplementsAndRunOutcome(t *testing.T) {
	content := "Captured Skill text."
	for _, test := range []struct {
		name, payload           string
		fail                    bool
		closed                  bool
		wantStage, code, output string
		facts                   OfflineInputs
	}{
		{name: "skill", payload: `{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Skill","tool_input":{"skill":"private"}}`, facts: OfflineInputs{SkillContent: &content}, wantStage: "complete", output: `"Captured Skill text."`},
		{name: "missing-skill", payload: `{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Skill","tool_input":{"skill":"private"},"tool_response":"native"}`, wantStage: "complete", output: `"native"`},
		{name: "session-start", payload: `{"session_id":"s","hook_event_name":"SessionStart","source":"startup","cwd":"/unavailable"}`, wantStage: "complete"},
		{name: "decode", payload: `{`, wantStage: "decode", code: "decode-error"},
		{name: "fail-open", payload: `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash"}`, fail: true, wantStage: "dispatch", code: "dispatch-error"},
		{name: "fail-closed", payload: `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash"}`, fail: true, closed: true, wantStage: "dispatch", code: "dispatch-error"},
		{name: "mcp-conflict", payload: `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"mcp__actual__read"}`, facts: OfflineInputs{MCP: &MCPIdentity{Server: "wrong", Tool: "read"}}, wantStage: "decode", code: "facts-conflict"},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := RunOutcome{}
			calls := 0
			policy := Policy{}
			if test.closed {
				policy = Policy{Fail: FailClosed, FailClosedExitCode: 1}
			}
			r := New(WithOfflineInputs(test.facts), WithPolicy(policy), WithRunObserver(func(value RunOutcome) { outcome = value; calls++ }), WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
			r.mcpWarmStart = func(string) { t.Error("external MCP warm") }
			r.codexMCPWarmStart = func(codexLaunchContext) { t.Error("external warm") }
			r.OnToolPost(func(ctx context.Context, event *ToolPostEvent) (ToolPostDecision, error) {
				if string(event.Output) != test.output {
					t.Errorf("output=%s want=%s", event.Output, test.output)
				}
				return Observed(), nil
			})
			r.OnToolPre(func(context.Context, *ToolPreEvent) (ToolPreDecision, error) {
				if test.fail {
					return NoDecision(), errors.New("SECRET ERROR")
				}
				return NoDecision(), nil
			})
			var out, errOut bytes.Buffer
			exit := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=claude-code"}, strings.NewReader(test.payload), &out, &errOut)
			if outcome.Stage != test.wantStage || outcome.ErrorCode != test.code || calls != 1 {
				t.Fatalf("outcome %+v calls=%d", outcome, calls)
			}
			if test.closed && exit != 1 {
				t.Fatalf("closed exit=%d", exit)
			}
			if strings.Contains(out.String()+errOut.String(), "SECRET ERROR") {
				t.Fatal("error text leaked")
			}
		})
	}
}

func TestM3A1OfflineSkillIgnoresExistingManifest(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, ".claude", "skills", "private", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("LOCAL_MANIFEST_CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"session_id": "s", "hook_event_name": "PostToolUse", "cwd": root, "tool_name": "Skill", "tool_input": map[string]string{"skill": "private"}, "tool_response": "native"}
	payload, _ := json.Marshal(input)
	for _, provided := range []*string{nil, new(string)} {
		r := New(WithOfflineInputs(OfflineInputs{SkillContent: provided}), WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
		calls := 0
		r.OnToolPost(func(_ context.Context, event *ToolPostEvent) (ToolPostDecision, error) {
			calls++
			want := `"native"`
			if provided != nil {
				want = `""`
			}
			if string(event.Output) != want {
				t.Errorf("offline decoder consumed the local manifest: %s", event.Output)
			}
			return Observed(), nil
		})
		if exit := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=claude-code"}, bytes.NewReader(payload), io.Discard, io.Discard); exit != 0 || calls != 1 {
			t.Fatalf("exit=%d calls=%d", exit, calls)
		}
	}
}

func TestM3A1OfflineMCPFillsOnlyMissingIdentity(t *testing.T) {
	for _, test := range []struct {
		name, tool, server, method string
		conflict                   bool
	}{
		{"unknown-tool", "remote_read", "server", "read", false},
		{"known-mcp", "mcp__server__read", "server", "read", false},
		{"conflicting-tool", "mcp__server__read", "server", "write", true},
		{"builtin-tool", "Bash", "server", "read", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := RunOutcome{}
			r := New(WithOfflineInputs(OfflineInputs{MCP: &MCPIdentity{Server: test.server, Tool: test.method}}), WithRunObserver(func(value RunOutcome) { outcome = value }))
			calls := 0
			r.OnToolPre(func(_ context.Context, event *ToolPreEvent) (ToolPreDecision, error) {
				calls++
				if event.Tool.MCP == nil || event.Tool.MCP.Server != test.server || event.Tool.MCP.Tool != test.method {
					t.Fatalf("MCP identity %+v", event.Tool.MCP)
				}
				return NoDecision(), nil
			})
			payload, _ := json.Marshal(map[string]any{"session_id": "s", "hook_event_name": "PreToolUse", "tool_name": test.tool})
			exit := r.Run(context.Background(), []string{"agenthooks", "run", "--provider=codex"}, bytes.NewReader(payload), io.Discard, io.Discard)
			if test.conflict {
				if exit != 1 || calls != 0 || outcome.ErrorCode != "facts-conflict" {
					t.Fatalf("conflict accepted: exit=%d calls=%d outcome=%+v", exit, calls, outcome)
				}
			} else if exit != 0 || calls != 1 || outcome.ErrorCode != "" {
				t.Fatalf("identity rejected: exit=%d calls=%d outcome=%+v", exit, calls, outcome)
			}
		})
	}
}
func TestM3A1NativeEventTable(t *testing.T) {
	if !RecognizesNativeEvent(ProviderCodex, "PreToolUse") || RecognizesNativeEvent(ProviderCodex, "UserPromptExpansion") || !RecognizesNativeEvent(ProviderClaudeCode, "UserPromptExpansion") || RecognizesNativeEvent(ProviderClaudeCode, "invented") {
		t.Fatal("codec event surface changed")
	}
}
