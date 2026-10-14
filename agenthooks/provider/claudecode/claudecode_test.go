package claudecode

import (
	"encoding/json"
	"testing"

	"github.com/speakeasy-api/agenthooks"
)

func TestPreToolUseViewWithExtraCapture(t *testing.T) {
	raw := `{"session_id":"s1","cwd":"/w","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"tool_use_id":"t1","brand_new_field":"surprise"}`
	e := &agenthooks.Event{
		Provider:   agenthooks.ProviderClaudeCode,
		NativeName: "PreToolUse",
		Kind:       agenthooks.KindToolPre,
		Raw:        json.RawMessage(raw),
	}
	v, ok := PreToolUse(e)
	if !ok {
		t.Fatal("view should decode")
	}
	if v.ToolName != "Bash" || v.SessionID != "s1" {
		t.Errorf("fields wrong: %+v", v)
	}
	if string(v.Extra["brand_new_field"]) != `"surprise"` {
		t.Errorf("unknown field must land in Extra, got %v", v.Extra)
	}

	if _, ok := PostToolUse(e); ok {
		t.Error("wrong native event must not decode")
	}
	e.Provider = agenthooks.ProviderCodex
	if _, ok := PreToolUse(e); ok {
		t.Error("wrong provider must not decode")
	}
}

func TestToolViewsExposeEffortMCPServerAndUnknownFields(t *testing.T) {
	tests := []struct {
		name string
		kind string
		view func(*agenthooks.Event) (string, string, string, map[string]json.RawMessage, map[string]json.RawMessage, bool)
	}{
		{
			name: "PreToolUse",
			kind: "PreToolUse",
			view: func(e *agenthooks.Event) (string, string, string, map[string]json.RawMessage, map[string]json.RawMessage, bool) {
				v, ok := PreToolUse(e)
				if !ok {
					return "", "", "", nil, nil, false
				}
				return v.Effort.Level, v.MCPServer.Name, v.MCPServer.Source, v.Extra, v.MCPServer.Extra, true
			},
		},
		{
			name: "PostToolUse",
			kind: "PostToolUse",
			view: func(e *agenthooks.Event) (string, string, string, map[string]json.RawMessage, map[string]json.RawMessage, bool) {
				v, ok := PostToolUse(e)
				if !ok {
					return "", "", "", nil, nil, false
				}
				return v.Effort.Level, v.MCPServer.Name, v.MCPServer.Source, v.Extra, v.MCPServer.Extra, true
			},
		},
		{
			name: "PermissionRequest",
			kind: "PermissionRequest",
			view: func(e *agenthooks.Event) (string, string, string, map[string]json.RawMessage, map[string]json.RawMessage, bool) {
				v, ok := PermissionRequest(e)
				if !ok {
					return "", "", "", nil, nil, false
				}
				return v.Effort.Level, v.MCPServer.Name, v.MCPServer.Source, v.Extra, v.MCPServer.Extra, true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"session_id":"s1","hook_event_name":"` + tt.kind + `","effort":{"level":"high","future_effort":true},"mcp_server":{"name":"docs","source":"plugin","future_server":"v2"},"future_top_level":"kept"}`
			e := &agenthooks.Event{
				Provider:   agenthooks.ProviderClaudeCode,
				NativeName: tt.kind,
				Raw:        json.RawMessage(raw),
			}
			effort, name, source, extra, mcpExtra, ok := tt.view(e)
			if !ok {
				t.Fatal("native view should decode")
			}
			if effort != "high" || name != "docs" || source != "plugin" {
				t.Fatalf("decoded effort/MCP fields = %q, %q, %q", effort, name, source)
			}
			if string(extra["future_top_level"]) != `"kept"` {
				t.Errorf("unknown top-level field not captured: %v", extra)
			}
			if string(mcpExtra["future_server"]) != `"v2"` {
				t.Errorf("unknown MCP server field not captured: %v", mcpExtra)
			}
		})
	}

	postFailure, ok := PostToolUseFailure(&agenthooks.Event{
		Provider:   agenthooks.ProviderClaudeCode,
		NativeName: "PostToolUseFailure",
		Raw:        json.RawMessage(`{"hook_event_name":"PostToolUseFailure","effort":{"level":"medium"}}`),
	})
	if !ok || postFailure.Effort.Level != "medium" {
		t.Fatalf("PostToolUseFailure effort view = %+v, %v", postFailure, ok)
	}
}

func TestUserPromptExpansionAndPostModelSwitchViews(t *testing.T) {
	expansionEvent := &agenthooks.Event{
		Provider:   agenthooks.ProviderClaudeCode,
		NativeName: "UserPromptExpansion",
		Raw:        json.RawMessage(`{"session_id":"s2","transcript_path":"/tmp/t.jsonl","cwd":"/work","hook_event_name":"UserPromptExpansion","permission_mode":"default","model":"sonnet","effort":{"level":"low"},"expansion_type":"command","command_name":"review","prompt":"expanded","future":"kept"}`),
	}
	expansion, ok := UserPromptExpansion(expansionEvent)
	if !ok {
		t.Fatal("UserPromptExpansion view should decode")
	}
	if expansion.SessionID != "s2" || expansion.TranscriptPath != "/tmp/t.jsonl" || expansion.CWD != "/work" || expansion.PermissionMode != "default" || expansion.Model != "sonnet" || expansion.Effort.Level != "low" || expansion.ExpansionType != "command" || expansion.CommandName != "review" || expansion.Prompt != "expanded" {
		t.Errorf("UserPromptExpansion fields wrong: %+v", expansion)
	}
	if string(expansion.Extra["future"]) != `"kept"` {
		t.Errorf("unknown expansion field not captured: %v", expansion.Extra)
	}
	if _, ok := PostModelSwitch(expansionEvent); ok {
		t.Error("wrong native event must not decode as PostModelSwitch")
	}

	switchEvent := &agenthooks.Event{
		Provider:   agenthooks.ProviderClaudeCode,
		NativeName: "PostModelSwitch",
		Raw:        json.RawMessage(`{"session_id":"s3","hook_event_name":"PostModelSwitch","from_model":"sonnet","to_model":"opus","future_switch":"kept"}`),
	}
	switchInput, ok := PostModelSwitch(switchEvent)
	if !ok {
		t.Fatal("PostModelSwitch view should decode")
	}
	if switchInput.SessionID != "s3" || switchInput.FromModel != "sonnet" || switchInput.ToModel != "opus" {
		t.Errorf("PostModelSwitch fields wrong: %+v", switchInput)
	}
	if string(switchInput.Extra["future_switch"]) != `"kept"` {
		t.Errorf("unknown switch field not captured: %v", switchInput.Extra)
	}
}

func TestSessionStartForkSourceView(t *testing.T) {
	e := &agenthooks.Event{
		Provider:   agenthooks.ProviderClaudeCode,
		NativeName: "SessionStart",
		Raw:        json.RawMessage(`{"session_id":"s4","hook_event_name":"SessionStart","source":"fork","future":"kept"}`),
	}
	v, ok := SessionStart(e)
	if !ok || v.Source != "fork" {
		t.Fatalf("SessionStart fork view = %+v, %v", v, ok)
	}
	if string(v.Extra["future"]) != `"kept"` {
		t.Errorf("unknown SessionStart field not captured: %v", v.Extra)
	}
}
