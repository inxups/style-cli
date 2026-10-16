package claude

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/speakeasy-api/agenthooks"
)

func TestEnricherStateUpdateUsesEventEnvironmentSessionPrecedence(t *testing.T) {
	enricher := &Enricher{Environment: func() string { return "low" }}
	event := claudeRuntimeEvent(agenthooks.KindSessionStart, "SessionStart", `{"session_id":"s1","cwd":"/repo","model":"sonnet","source":"startup","effort":{"level":"high"}}`)
	start := &agenthooks.SessionStartEvent{Event: *event, Source: "startup"}
	update := enricher.StateUpdate(event, start, state.Session{})
	if update.Source != "startup" || update.Model != "sonnet" || update.ThinkingEffort != "high" || update.EffortRaw != "high" || !update.EffortResolved || update.EffortApprox == nil || *update.EffortApprox {
		t.Fatalf("event effort update = %#v", update)
	}

	// A later event value remains authoritative after an earlier resolution.
	next := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s1","effort":{"level":"max"}}`)
	next.Session.EffortRaw = "max"
	later := enricher.StateUpdate(next, &agenthooks.ToolPreEvent{Event: *next}, state.Session{EffortResolved: true, ThinkingEffort: "high", EffortRaw: "high"})
	if later.ThinkingEffort != "max" || later.EffortRaw != "max" || !later.EffortResolved {
		t.Fatalf("later event effort update = %#v", later)
	}

	// With neither an event value nor CLAUDE_EFFORT, the persisted session
	// value is kept.
	enricher.Environment = func() string { return "" }
	sessionEvent := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s1"}`)
	sessionOnly := enricher.StateUpdate(sessionEvent, &agenthooks.ToolPreEvent{Event: *sessionEvent}, state.Session{EffortResolved: false, ThinkingEffort: "medium", EffortRaw: "medium"})
	if sessionOnly.ThinkingEffort != "medium" || sessionOnly.EffortRaw != "medium" || !sessionOnly.EffortResolved {
		t.Fatalf("session effort update = %#v", sessionOnly)
	}
}

func TestResolvedSessionStillReadsClaudeEffortEnvironment(t *testing.T) {
	enricher := &Enricher{Environment: func() string { return "low" }}
	event := claudeRuntimeEvent(agenthooks.KindPromptSubmitted, "UserPromptSubmit", `{"session_id":"s-env","prompt_id":"p2","prompt":"next"}`)
	previous := state.Session{EffortResolved: true, ThinkingEffort: "high", EffortRaw: "high"}
	update := enricher.StateUpdate(event, &agenthooks.PromptEvent{Event: *event, Prompt: "next"}, previous)
	if update.ThinkingEffort != "low" || update.EffortRaw != "low" || !update.EffortResolved {
		t.Fatalf("resolved session ignored CLAUDE_EFFORT: %#v", update)
	}

	// The event value still wins over the environment.
	tool := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s-env","effort":{"level":"max"}}`)
	tool.Session.EffortRaw = "max"
	if update := enricher.StateUpdate(tool, &agenthooks.ToolPreEvent{Event: *tool}, previous); update.ThinkingEffort != "max" || update.EffortRaw != "max" {
		t.Fatalf("event effort lost to environment: %#v", update)
	}
}

func TestEnricherStateUpdateKeepsUnknownEventRaw(t *testing.T) {
	event := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s2","effort":{"level":"future-tier"}}`)
	event.Session.EffortRaw = "future-tier"
	update := (&Enricher{Environment: func() string { return "high" }}).StateUpdate(event, event, state.Session{})
	if !update.EffortResolved || update.ThinkingEffort != "" || update.EffortRaw != "future-tier" || update.EffortApprox != nil {
		t.Fatalf("unknown event effort = %#v", update)
	}
}

func TestUnknownEventEffortClearsPersistedTier(t *testing.T) {
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	enricher := &Enricher{Environment: func() string { return "" }}
	for _, level := range []string{"high", "future-tier"} {
		event := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s-stale","effort":{"level":"`+level+`"}}`)
		event.Session.EffortRaw = level
		previous, err := store.ReadSession("s-stale")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Update("s-stale", "", enricher.StateUpdate(event, &agenthooks.ToolPreEvent{Event: *event}, previous)); err != nil {
			t.Fatal(err)
		}
	}
	session, err := store.ReadSession("s-stale")
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := when.Compile(when.Rule{ThinkingEffort: []string{"high"}})
	if err != nil {
		t.Fatal(err)
	}
	if session.ThinkingEffort != "" || session.EffortRaw != "future-tier" || matcher.Match(when.Input{ThinkingEffort: session.ThinkingEffort}) {
		t.Fatalf("unknown effort left stale tier: %#v", session)
	}
}

func TestEnricherStateUpdateProjectsPostModelSwitch(t *testing.T) {
	event := claudeRuntimeEvent(agenthooks.KindOther, "PostModelSwitch", `{"session_id":"s-model","to_model":"opus"}`)
	event.Session.Model = "opus"
	enricher := &Enricher{HarnessVersion: "2.1.315", Environment: func() string { return "" }}
	update := enricher.StateUpdate(event, &agenthooks.ModelSwitchEvent{Event: *event, ToModel: "opus"}, state.Session{EffortResolved: true, ThinkingEffort: "high", EffortRaw: "high"})
	// The switch updates the model; effort falls back to the persisted value
	// unchanged because neither the event nor CLAUDE_EFFORT reports one.
	if update.Model != "opus" || update.HarnessVersion != "2.1.315" || update.ThinkingEffort != "high" || update.EffortRaw != "high" {
		t.Fatalf("model switch update = %#v", update)
	}
}

func TestEnricherCreatesPromptExpansionPoint(t *testing.T) {
	event := claudeRuntimeEvent(agenthooks.KindPromptExpansion, "UserPromptExpansion", `{"session_id":"s3","prompt_id":"p3","cwd":"/repo","permission_mode":"default","expansion_type":"slash_command","command_name":"review","prompt":"/review"}`)
	expansion := &agenthooks.PromptExpansionEvent{Event: *event, ExpansionType: "slash_command", CommandName: "review", Prompt: "/review"}
	input, limit, err := NewEnricher().Enrich(event, expansion, state.Session{Model: "sonnet", ThinkingEffort: "high"}, state.Scope{SessionID: "s3", AgentID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if limit.Value != 10000 || limit.Unit != "chars" || input.Harness != string(agenthooks.ProviderClaudeCode) || input.TurnID != "p3" {
		t.Fatalf("prompt expansion envelope = %#v limit=%#v", input, limit)
	}
	if len(input.Points) != 1 || input.Points[0].Name != plan.SkillExplicit || input.Points[0].SkillName != "review" {
		t.Fatalf("prompt expansion points = %#v", input.Points)
	}
	point := input.Points[0]
	if point.Values["skill.name"] != "review" || point.Values["prompt"] != "/review" || point.When.Harness["permission_mode"] != "default" {
		t.Fatalf("prompt expansion values = %#v when=%#v", point.Values, point.When)
	}
	if got, ok := point.When.Fields.ReadConditionField("skill.name"); !ok || got != "review" {
		t.Fatalf("skill name condition = %q, %v", got, ok)
	}

	other := *event
	other.Raw = json.RawMessage(`{"session_id":"s3","hook_event_name":"UserPromptExpansion","expansion_type":"other","command_name":"review"}`)
	otherTyped := &agenthooks.PromptExpansionEvent{Event: other, ExpansionType: "other", CommandName: "review"}
	otherInput, _, err := NewEnricher().Enrich(&other, otherTyped, state.Session{}, state.Scope{SessionID: "s3", AgentID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(otherInput.Points) != 0 {
		t.Fatalf("non-slash expansion should not make explicit skill point: %#v", otherInput.Points)
	}
}

func TestEnricherMapsClaudeToolFieldsAddedMCPAndOutput(t *testing.T) {
	t.Run("Edit added text", func(t *testing.T) {
		event := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s4","cwd":"/repo","permission_mode":"default","tool_name":"Edit","tool_input":{"file_path":"x.go","old_string":"old","new_string":"new TODO"}}`)
		tool := agenthooks.ToolCall{Name: "Edit", Canonical: agenthooks.ToolFileEdit, Input: json.RawMessage(`{"file_path":"x.go","old_string":"old","new_string":"new TODO"}`)}
		pre := &agenthooks.ToolPreEvent{Event: *event, Tool: tool}
		input, _, err := NewEnricher().Enrich(event, pre, state.Session{}, state.Scope{SessionID: "s4", AgentID: "main"})
		if err != nil {
			t.Fatal(err)
		}
		point := input.Points[0]
		if point.Name != plan.ToolPre || point.ToolName != "Edit" || point.CanonicalTool != "file.edit" {
			t.Fatalf("Edit point = %#v", point)
		}
		for field, want := range map[string]string{"tool.added": "new TODO", "tool.input.file_path": "x.go", "tool.input.new_string": "new TODO", "cwd": "/repo"} {
			if got, ok := point.When.Fields.ReadConditionField(field); !ok || got != want {
				t.Errorf("field %q = %q, %v; want %q", field, got, ok, want)
			}
		}
	})

	t.Run("Write MCP output", func(t *testing.T) {
		event := claudeRuntimeEvent(agenthooks.KindToolPost, "PostToolUse", `{"session_id":"s5","tool_name":"mcp__calendar__create_event","tool_input":{"title":"holiday","nested":{"ok":true}},"tool_response":{"created":true}}`)
		tool := agenthooks.ToolCall{Name: "mcp__calendar__create_event", Canonical: agenthooks.ToolMCP, Input: json.RawMessage(`{"title":"holiday","nested":{"ok":true}}`), MCP: &agenthooks.MCPCall{Server: "calendar", Tool: "create_event"}}
		post := &agenthooks.ToolPostEvent{Event: *event, Tool: tool, Output: json.RawMessage(`{"created":true}`)}
		input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "s5", AgentID: "main"})
		if err != nil {
			t.Fatal(err)
		}
		point := input.Points[0]
		if point.Name != plan.ToolPost || point.MCPServer != "calendar" || point.MCPTool != "create_event" {
			t.Fatalf("MCP point = %#v", point)
		}
		for field, want := range map[string]string{"mcp.server": "calendar", "mcp.tool": "create_event", "tool.output": `{"created":true}`, "tool.input.nested.ok": "true"} {
			if got, ok := point.When.Fields.ReadConditionField(field); !ok || got != want {
				t.Errorf("field %q = %q, %v; want %q", field, got, ok, want)
			}
		}
	})

	t.Run("Write added text", func(t *testing.T) {
		event := claudeRuntimeEvent(agenthooks.KindToolError, "PostToolUseFailure", `{"session_id":"s6","tool_name":"Write","tool_input":{"file_path":"x.txt","content":"body"},"tool_error":"permission denied"}`)
		tool := agenthooks.ToolCall{Name: "Write", Canonical: agenthooks.ToolFileWrite, Input: json.RawMessage(`{"file_path":"x.txt","content":"body"}`)}
		post := &agenthooks.ToolPostEvent{Event: *event, Tool: tool, Failed: true, Error: "permission denied"}
		input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "s6", AgentID: "main"})
		if err != nil {
			t.Fatal(err)
		}
		point := input.Points[0]
		if point.Name != plan.ToolError || point.Values["tool.added"] != "body" || point.Values["tool.output"] != "permission denied" {
			t.Fatalf("tool error point = %#v", point)
		}
	})
}

func TestEnricherProjectsImplicitSkillAndSubagentScope(t *testing.T) {
	event := claudeRuntimeEvent(agenthooks.KindToolPost, "PostToolUse", `{"session_id":"s7","tool_name":"Skill","tool_input":{"skill":"review"},"tool_response":"manifest"}`)
	tool := agenthooks.ToolCall{Name: "Skill", Canonical: agenthooks.ToolOther, Input: json.RawMessage(`{"skill":"review"}`)}
	post := &agenthooks.ToolPostEvent{Event: *event, Tool: tool, Output: json.RawMessage(`"manifest"`)}
	input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "s7", AgentID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if input.Points[0].SkillName != "review" || input.Points[0].Values["skill.name"] != "review" {
		t.Fatalf("implicit skill point = %#v", input.Points[0])
	}

	subagent := claudeRuntimeEvent(agenthooks.KindSubagentStart, "SubagentStart", `{"session_id":"s7","agent_id":"child-1","agent_type":"Explore"}`)
	subagent.Agent = &agenthooks.AgentInfo{ID: "child-1", Type: "Explore"}
	childInput, _, err := NewEnricher().Enrich(subagent, &agenthooks.SubagentStartEvent{Event: *subagent}, state.Session{}, state.Scope{SessionID: "s7", AgentID: "child-1", PromptCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if childInput.Scope.AgentID != "child-1" || childInput.Points[0].When.AgentType != "Explore" || childInput.Points[0].When.Harness["agent"] != "Explore" {
		t.Fatalf("subagent projection = %#v", childInput)
	}
	if childInput.Points[0].Capabilities.AddContext {
		t.Fatal("Claude SubagentStart must not advertise add-context")
	}
}

func TestEnrichKeepsShellCharactersInToolInput(t *testing.T) {
	raw := `{"command":"cd repo && git push > out.log 2<&1","nested":{"q":"a&b<c>"}}`
	event := claudeRuntimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s9","tool_name":"Bash","tool_input":`+raw+`}`)
	pre := &agenthooks.ToolPreEvent{Event: *event, Tool: agenthooks.ToolCall{Name: "Bash", Input: json.RawMessage(raw)}}
	input, _, err := NewEnricher().Enrich(event, pre, state.Session{}, state.Scope{SessionID: "s9", AgentID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	point := input.Points[0]
	for field, want := range map[string]string{
		"tool.input":         `{"command":"cd repo && git push > out.log 2<&1","nested":{"q":"a&b<c>"}}`,
		"tool.input.nested":  `{"q":"a&b<c>"}`,
		"tool.input.command": "cd repo && git push > out.log 2<&1",
	} {
		if got, ok := point.When.Fields.ReadConditionField(field); !ok || got != want {
			t.Errorf("condition field %q = %q, %v; want %q", field, got, ok, want)
		}
	}
}

func claudeRuntimeEvent(kind agenthooks.EventKind, nativeName, raw string) *agenthooks.Event {
	var fields struct {
		ID             string `json:"session_id"`
		TurnID         string `json:"prompt_id"`
		CWD            string `json:"cwd"`
		Model          string `json:"model"`
		PermissionMode string `json:"permission_mode"`
		Effort         struct {
			Level string `json:"level"`
		} `json:"effort"`
	}
	_ = json.Unmarshal([]byte(raw), &fields)
	return &agenthooks.Event{
		Provider: agenthooks.ProviderClaudeCode, NativeName: nativeName, Kind: kind,
		Session: agenthooks.SessionInfo{ID: fields.ID, TurnID: fields.TurnID, CWD: fields.CWD, Model: fields.Model, PermissionMode: fields.PermissionMode, EffortRaw: fields.Effort.Level},
		Raw:     json.RawMessage(raw),
	}
}

func TestEnricherRejectsMismatchedEventsAndProviders(t *testing.T) {
	enricher := NewEnricher()
	event := claudeRuntimeEvent(agenthooks.KindPromptSubmitted, "UserPromptSubmit", `{}`)
	if _, _, err := enricher.Enrich(event, &agenthooks.ToolPreEvent{Event: *event}, state.Session{}, state.Scope{}); err == nil {
		t.Fatal("expected a typed-event mismatch error")
	}
	event.Provider = agenthooks.ProviderCodex
	if _, _, err := enricher.Enrich(event, event, state.Session{}, state.Scope{}); err == nil {
		t.Fatal("expected a wrong-provider error")
	}
}

func TestEnricherVariablesIncludeClaudeHarnessFacts(t *testing.T) {
	event := claudeRuntimeEvent(agenthooks.KindPromptSubmitted, "UserPromptSubmit", `{"session_id":"s8","prompt_id":"turn-8","cwd":"/repo","permission_mode":"plan","model":"opus"}`)
	event.Agent = &agenthooks.AgentInfo{ID: "child-8", Type: "Explore"}
	input, _, err := NewEnricher().Enrich(event, &agenthooks.PromptEvent{Event: *event, Prompt: "hello"}, state.Session{HarnessVersion: "2.1.1", ThinkingEffort: "high", EffortRaw: "HIGH"}, state.Scope{SessionID: "s8", AgentID: "child-8"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"model": "opus", "thinking_effort": "high", "thinking_effort_raw": "HIGH", "harness.name": "claude-code", "harness.cwd": "/repo", "harness.permission_mode": "plan", "harness.session_id": "s8", "harness.turn_id": "turn-8", "harness.version": "2.1.1", "harness.agent": "Explore"}
	if !reflect.DeepEqual(input.Variables, want) {
		t.Fatalf("variables = %#v, want %#v", input.Variables, want)
	}
	if !strings.Contains(input.Points[0].Values["prompt"], "hello") {
		t.Fatalf("prompt values = %#v", input.Points[0].Values)
	}
}
