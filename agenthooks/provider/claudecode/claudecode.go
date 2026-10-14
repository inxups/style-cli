// Package claudecode provides complete typed views over native Claude Code
// hook payloads. The unified layer in the root package is convenience; this
// typed native layer is the fidelity guarantee: every struct captures
// unrecognized JSON keys in Extra, so provider drift never drops data.
//
// Structs are maintained against the Claude Code hooks documentation with
// golden fixtures (DESIGN.md §10).
package claudecode

import (
	"encoding/json"

	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/internal/jsonx"
)

// PreToolUseInput is the native PreToolUse payload.
type PreToolUseInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	PermissionMode string                     `json:"permission_mode"`
	Effort         Effort                     `json:"effort"`
	MCPServer      *MCPServerInfo             `json:"mcp_server"`
	ToolName       string                     `json:"tool_name"`
	ToolInput      json.RawMessage            `json:"tool_input"`
	ToolUseID      string                     `json:"tool_use_id"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// PostToolUseInput is the native PostToolUse payload (and, with the failure
// fields set, PostToolUseFailure).
type PostToolUseInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	PermissionMode string                     `json:"permission_mode"`
	Effort         Effort                     `json:"effort"`
	MCPServer      *MCPServerInfo             `json:"mcp_server"`
	ToolName       string                     `json:"tool_name"`
	ToolInput      json.RawMessage            `json:"tool_input"`
	ToolUseID      string                     `json:"tool_use_id"`
	ToolResponse   json.RawMessage            `json:"tool_response"`
	ToolError      string                     `json:"tool_error"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// PermissionRequestInput is the native PermissionRequest payload.
type PermissionRequestInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	PermissionMode string                     `json:"permission_mode"`
	Effort         Effort                     `json:"effort"`
	MCPServer      *MCPServerInfo             `json:"mcp_server"`
	ToolName       string                     `json:"tool_name"`
	ToolInput      json.RawMessage            `json:"tool_input"`
	ToolUseID      string                     `json:"tool_use_id"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// Effort is Claude Code's native effort object. Extra preserves fields added
// by future provider versions.
type Effort struct {
	Level string                     `json:"level"`
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON retains unknown nested effort fields as well as Level.
func (e *Effort) UnmarshalJSON(data []byte) error {
	type effortAlias Effort
	var decoded effortAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	delete(all, "level")
	if len(all) > 0 {
		decoded.Extra = all
	}
	*e = Effort(decoded)
	return nil
}

// MCPServerInfo describes the MCP server named by a Claude Code hook.
// Extra retains provider additions to this nested object.
type MCPServerInfo struct {
	Name   string                     `json:"name"`
	Source string                     `json:"source"`
	Extra  map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON retains unknown nested MCP server fields.
func (m *MCPServerInfo) UnmarshalJSON(data []byte) error {
	type mcpServerAlias MCPServerInfo
	var decoded mcpServerAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	delete(all, "name")
	delete(all, "source")
	if len(all) > 0 {
		decoded.Extra = all
	}
	*m = MCPServerInfo(decoded)
	return nil
}

// UserPromptSubmitInput is the native UserPromptSubmit payload.
type UserPromptSubmitInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	PermissionMode string                     `json:"permission_mode"`
	Prompt         string                     `json:"prompt"`
	PromptID       string                     `json:"prompt_id"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// UserPromptExpansionInput is the native UserPromptExpansion payload.
type UserPromptExpansionInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	PermissionMode string                     `json:"permission_mode"`
	Model          string                     `json:"model"`
	Effort         Effort                     `json:"effort"`
	ExpansionType  string                     `json:"expansion_type"`
	CommandName    string                     `json:"command_name"`
	Prompt         string                     `json:"prompt"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// PostModelSwitchInput is the native PostModelSwitch payload.
type PostModelSwitchInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	PermissionMode string                     `json:"permission_mode"`
	Model          string                     `json:"model"`
	Effort         Effort                     `json:"effort"`
	FromModel      string                     `json:"from_model"`
	ToModel        string                     `json:"to_model"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// StopInput is the native Stop / SubagentStop payload.
type StopInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	StopHookActive bool                       `json:"stop_hook_active"`
	AgentID        string                     `json:"agent_id"`
	AgentType      string                     `json:"agent_type"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// SessionStartInput is the native SessionStart payload.
type SessionStartInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	Source         string                     `json:"source"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// SessionEndInput is the native SessionEnd payload.
type SessionEndInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	Reason         string                     `json:"reason"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// NotificationInput is the native Notification payload.
type NotificationInput struct {
	SessionID      string                     `json:"session_id"`
	TranscriptPath string                     `json:"transcript_path"`
	CWD            string                     `json:"cwd"`
	HookEventName  string                     `json:"hook_event_name"`
	Message        string                     `json:"message"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// PreCompactInput is the native PreCompact / PostCompact payload.
type PreCompactInput struct {
	SessionID          string                     `json:"session_id"`
	TranscriptPath     string                     `json:"transcript_path"`
	CWD                string                     `json:"cwd"`
	HookEventName      string                     `json:"hook_event_name"`
	Trigger            string                     `json:"trigger"`
	CustomInstructions string                     `json:"custom_instructions"`
	Extra              map[string]json.RawMessage `json:"-"`
}

// FileChangedInput is the native FileChanged payload.
type FileChangedInput struct {
	SessionID     string                     `json:"session_id"`
	CWD           string                     `json:"cwd"`
	HookEventName string                     `json:"hook_event_name"`
	FilePath      string                     `json:"file_path"`
	Extra         map[string]json.RawMessage `json:"-"`
}

func view[T any](e *agenthooks.Event, native string) (*T, bool) {
	if e == nil || e.Provider != agenthooks.ProviderClaudeCode || e.NativeName != native {
		return nil, false
	}
	var v T
	if err := jsonx.Unmarshal(e.Raw, &v); err != nil {
		return nil, false
	}
	return &v, true
}

// Typed views: nil,false when the event is a different provider/native event.

func PreToolUse(e *agenthooks.Event) (*PreToolUseInput, bool) {
	return view[PreToolUseInput](e, "PreToolUse")
}

func PostToolUse(e *agenthooks.Event) (*PostToolUseInput, bool) {
	return view[PostToolUseInput](e, "PostToolUse")
}

func PostToolUseFailure(e *agenthooks.Event) (*PostToolUseInput, bool) {
	return view[PostToolUseInput](e, "PostToolUseFailure")
}

func PermissionRequest(e *agenthooks.Event) (*PermissionRequestInput, bool) {
	return view[PermissionRequestInput](e, "PermissionRequest")
}

func UserPromptSubmit(e *agenthooks.Event) (*UserPromptSubmitInput, bool) {
	return view[UserPromptSubmitInput](e, "UserPromptSubmit")
}

func UserPromptExpansion(e *agenthooks.Event) (*UserPromptExpansionInput, bool) {
	return view[UserPromptExpansionInput](e, "UserPromptExpansion")
}

func PostModelSwitch(e *agenthooks.Event) (*PostModelSwitchInput, bool) {
	return view[PostModelSwitchInput](e, "PostModelSwitch")
}

func Stop(e *agenthooks.Event) (*StopInput, bool) {
	return view[StopInput](e, "Stop")
}

func SubagentStop(e *agenthooks.Event) (*StopInput, bool) {
	return view[StopInput](e, "SubagentStop")
}

func SessionStart(e *agenthooks.Event) (*SessionStartInput, bool) {
	return view[SessionStartInput](e, "SessionStart")
}

func SessionEnd(e *agenthooks.Event) (*SessionEndInput, bool) {
	return view[SessionEndInput](e, "SessionEnd")
}

func Notification(e *agenthooks.Event) (*NotificationInput, bool) {
	return view[NotificationInput](e, "Notification")
}

func PreCompact(e *agenthooks.Event) (*PreCompactInput, bool) {
	return view[PreCompactInput](e, "PreCompact")
}

func PostCompact(e *agenthooks.Event) (*PreCompactInput, bool) {
	return view[PreCompactInput](e, "PostCompact")
}

func FileChanged(e *agenthooks.Event) (*FileChangedInput, bool) {
	return view[FileChangedInput](e, "FileChanged")
}
