package codex

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks"
)

func TestEnricherStateUpdateProjectsFactsAndResolvesEffortOnce(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "repo")
	home := filepath.Join(root, "codex-home")
	reads := make(map[string]int)
	enricher := &Enricher{
		Home: home,
		ProjectTrusted: func(cwd string) bool {
			if cwd != project {
				t.Fatalf("trusted-project check cwd = %q, want %q", cwd, project)
			}
			return true
		},
		LaunchSettings: func(cwd string) (agenthooks.CodexLaunchSettings, bool) {
			return agenthooks.CodexLaunchSettings{}, true
		},
		ReadFile: func(path string) ([]byte, error) {
			reads[path]++
			if path == filepath.Join(project, ".codex", "config.toml") {
				return []byte(`model_reasoning_effort = "ultra"`), nil
			}
			return nil, filepath.SkipAll
		},
	}
	event := &agenthooks.Event{
		Provider: agenthooks.ProviderCodex,
		Kind:     agenthooks.KindSessionStart,
		Session:  agenthooks.SessionInfo{ID: "session-1", CWD: project, Model: "gpt-5", HarnessVersion: "0.146.0"},
		Raw:      json.RawMessage(`{"session_id":"session-1","cwd":"` + project + `","source":"startup"}`),
	}
	start := &agenthooks.SessionStartEvent{Event: *event, Source: "startup"}
	update := enricher.StateUpdate(event, start, state.Session{})
	if update.Model != "gpt-5" || update.HarnessVersion != "0.146.0" || update.Source != "startup" {
		t.Fatalf("session facts = %#v", update)
	}
	if !update.EffortResolved || update.ThinkingEffort != "max" || update.EffortRaw != "ultra" || update.EffortApprox == nil || *update.EffortApprox {
		t.Fatalf("resolved effort = %#v", update)
	}
	if reads[filepath.Join(project, ".codex", "config.toml")] != 1 {
		t.Fatalf("project config reads = %d, want 1", reads[filepath.Join(project, ".codex", "config.toml")])
	}
	if reads[filepath.Join(home, "config.toml")] != 1 {
		t.Fatalf("user config reads = %d, want 1", reads[filepath.Join(home, "config.toml")])
	}

	launchCalls, trustCalls := 0, 0
	enricher.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) {
		launchCalls++
		return agenthooks.CodexLaunchSettings{}, true
	}
	enricher.ProjectTrusted = func(string) bool { trustCalls++; return false }
	next := *event
	next.Kind = agenthooks.KindToolPre
	next.Session.Model = "gpt-5-codex"
	next.Raw = json.RawMessage(`{"session_id":"session-1","hook_event_name":"PreToolUse"}`)
	second := enricher.StateUpdate(&next, &agenthooks.ToolPreEvent{Event: next}, state.Session{EffortResolved: true})
	if second.Model != "gpt-5-codex" || second.EffortResolved || second.EffortRaw != "" {
		t.Fatalf("subsequent state update = %#v", second)
	}
	if launchCalls != 0 || trustCalls != 0 {
		t.Fatalf("resolved session reread config inputs: launch=%d trust=%d", launchCalls, trustCalls)
	}
}

func TestEnricherStateUpdateKeepsUnknownEffortRaw(t *testing.T) {
	event := &agenthooks.Event{
		Provider: agenthooks.ProviderCodex,
		Session:  agenthooks.SessionInfo{ID: "session-unknown"},
	}
	enricher := &Enricher{LaunchSettings: func(string) (agenthooks.CodexLaunchSettings, bool) {
		return agenthooks.CodexLaunchSettings{ModelReasoningEffort: "future-tier"}, true
	}}
	update := enricher.StateUpdate(event, event, state.Session{})
	if !update.EffortResolved || update.ThinkingEffort != "" || update.EffortRaw != "future-tier" || update.EffortApprox != nil {
		t.Fatalf("unknown effort update = %#v", update)
	}
}

func TestEnricherSelectsFileProfileFromInstalledHarnessVersion(t *testing.T) {
	home := t.TempDir()
	enricher := &Enricher{Home: home, HarnessVersion: "0.159.3",
		LaunchSettings: func(string) (agenthooks.CodexLaunchSettings, bool) {
			return agenthooks.CodexLaunchSettings{Profile: "probe"}, true
		},
		ReadFile: func(path string) ([]byte, error) {
			if path == filepath.Join(home, "probe.config.toml") {
				return []byte(`model_reasoning_effort="high"`), nil
			}
			return []byte(`model_reasoning_effort="low"`), nil
		},
	}
	update := enricher.StateUpdate(&agenthooks.Event{Provider: agenthooks.ProviderCodex}, nil, state.Session{})
	if !update.EffortResolved || update.ThinkingEffort != "high" || update.HarnessVersion != "0.159.3" {
		t.Fatalf("installed version did not select file profile: %+v", update)
	}
}

func TestEnricherCreatesSessionStartAndZeroTurnFallback(t *testing.T) {
	event := runtimeEvent(agenthooks.KindSessionStart, "SessionStart", `{"session_id":"session-start","cwd":"/repo","source":"startup"}`)
	input, limit, err := NewEnricher().Enrich(event, &agenthooks.SessionStartEvent{Event: *event, Source: "startup"}, state.Session{}, state.Scope{
		SessionID: "session-start", AgentID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.TurnID != "0" || input.Variables["harness.turn_id"] != "0" || limit.Value != 2500 {
		t.Fatalf("session start turn/limit = %q / %#v / %#v", input.TurnID, input.Variables, limit)
	}
	if len(input.Points) != 1 || input.Points[0].Name != "session.start" || !input.Points[0].Capabilities.AddContext || input.Points[0].Capabilities.Deny {
		t.Fatalf("session-start points = %#v", input.Points)
	}
	if got, ok := input.Points[0].When.Fields.ReadConditionField("cwd"); !ok || got != "/repo" {
		t.Fatalf("session-start cwd = %q, %v", got, ok)
	}
}

func TestEnricherCreatesPromptAndExplicitSkillPoints(t *testing.T) {
	event := runtimeEvent(agenthooks.KindPromptSubmitted, "UserPromptSubmit", `{"session_id":"s1","turn_id":"t7","cwd":"/repo","prompt":"Use $reviewer then $lint and $reviewer; ignore $$nested and prefix$name."}`)
	prompt := &agenthooks.PromptEvent{Event: *event, Prompt: "Use $reviewer then $lint and $reviewer; ignore $$nested and prefix$name."}
	enricher := NewEnricher()
	input, limit, err := enricher.Enrich(event, prompt, state.Session{Model: "gpt-5", ThinkingEffort: "high", EffortRaw: "HIGH"}, state.Scope{SessionID: "s1", AgentID: "main", PromptCount: 4})
	if err != nil {
		t.Fatal(err)
	}
	if input.Harness != "codex" || input.TurnID != "t7" || input.Scope.AgentID != "main" || limit.Value != 2500 || limit.Unit != "tokens" {
		t.Fatalf("runtime envelope = %#v, limit=%#v", input, limit)
	}
	wantPoints := []string{"prompt.submitted", "skill.explicit", "skill.explicit"}
	gotPoints := make([]string, len(input.Points))
	for i, point := range input.Points {
		gotPoints[i] = point.Name
	}
	if !reflect.DeepEqual(gotPoints, wantPoints) {
		t.Fatalf("point order = %#v, want %#v", gotPoints, wantPoints)
	}
	if input.Points[0].Values["prompt"] != prompt.Prompt || input.Points[0].When.Model != "gpt-5" || input.Points[0].When.ThinkingEffort != "high" {
		t.Fatalf("prompt point facts = %#v", input.Points[0])
	}
	for i, want := range []string{"reviewer", "lint"} {
		point := input.Points[i+1]
		if point.SkillName != want || point.Values["skill.name"] != want || point.When.Fields == nil {
			t.Errorf("skill point %d = %#v, want %q", i, point, want)
			continue
		}
		if got, ok := point.When.Fields.ReadConditionField("prompt"); !ok || got != prompt.Prompt {
			t.Errorf("skill prompt condition field = %q, %v", got, ok)
		}
		if got, ok := point.When.Fields.ReadConditionField("skill.name"); !ok || got != want {
			t.Errorf("skill.name condition field = %q, %v", got, ok)
		}
	}
}

func TestEnricherMapsToolInputPatchFieldsAndMCP(t *testing.T) {
	t.Run("apply patch added lines", func(t *testing.T) {
		event := runtimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s2","turn_id":"t8","cwd":"/repo","permission_mode":"on-request","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: x.go\n@@\n-old\n+new TODO\n+++count; // FIXME\n*** End Patch\n"}}`)
		tool := agenthooks.ToolCall{
			Name: "apply_patch", Canonical: agenthooks.ToolFileEdit,
			Input:    json.RawMessage(`{"command":"*** Begin Patch\n*** Update File: x.go\n@@\n-old\n+new TODO\n+++count; // FIXME\n*** End Patch\n"}`),
			RawInput: json.RawMessage(`{"command":"*** Begin Patch\n*** Update File: x.go\n@@\n-old\n+new TODO\n+++count; // FIXME\n*** End Patch\n"}`),
		}
		pre := &agenthooks.ToolPreEvent{Event: *event, Tool: tool}
		input, _, err := NewEnricher().Enrich(event, pre, state.Session{Model: "gpt-5"}, state.Scope{SessionID: "s2", AgentID: "main", PromptCount: 8})
		if err != nil {
			t.Fatal(err)
		}
		point := input.Points[0]
		if point.Name != "tool.pre" || point.ToolName != "apply_patch" || point.CanonicalTool != "file.edit" || !point.Capabilities.Deny || !point.Capabilities.AddContext {
			t.Fatalf("tool point = %#v", point)
		}
		if got, ok := point.When.Fields.ReadConditionField("tool.added"); !ok || got != "new TODO\n++count; // FIXME" {
			t.Fatalf("tool.added = %q, %v; want new additions only", got, ok)
		}
		if got, ok := point.When.Fields.ReadConditionField("tool.input.command"); !ok || !strings.Contains(got, "+new TODO") {
			t.Fatalf("nested input condition = %q, %v", got, ok)
		}
		if point.Values["tool.input.command"] == "" || input.Variables["harness.permission_mode"] != "on-request" || input.Variables["harness.cwd"] != "/repo" {
			t.Fatalf("tool template/input values = %#v / %#v", point.Values, input.Variables)
		}
		if input.TurnID != "t8" || point.When.Harness["name"] != "codex" || point.When.Harness["agent"] != "main" {
			t.Fatalf("tool scope/condition facts = %#v / %#v", input, point.When)
		}
	})

	t.Run("MCP identity and nested JSON fields", func(t *testing.T) {
		event := runtimeEvent(agenthooks.KindToolPost, "PostToolUse", `{"session_id":"s3","tool_name":"mcp__calendar__create_event","tool_input":{"nested":{"title":"holiday","tags":["work","personal"]}},"tool_response":{"ok":true}}`)
		tool := agenthooks.ToolCall{
			Name: "mcp__calendar__create_event", Canonical: agenthooks.ToolMCP,
			MCP:      &agenthooks.MCPCall{Server: "calendar", Tool: "create_event"},
			Input:    json.RawMessage(`{"nested":{"title":"holiday","tags":["work","personal"]}}`),
			RawInput: json.RawMessage(`{"nested":{"title":"holiday","tags":["work","personal"]}}`),
		}
		post := &agenthooks.ToolPostEvent{
			Event: *event, Tool: tool,
			Output: json.RawMessage(`{"ok":true}`),
		}
		input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "s3", AgentID: "main"})
		if err != nil {
			t.Fatal(err)
		}
		point := input.Points[0]
		if point.Name != "tool.post" || point.MCPServer != "calendar" || point.MCPTool != "create_event" {
			t.Fatalf("MCP point = %#v", point)
		}
		for field, want := range map[string]string{
			"mcp.server": "calendar", "mcp.tool": "create_event",
			"tool.input.nested.title": "holiday", "tool.output": `{"ok":true}`, "tool.output.ok": "true",
			"tool.input.nested":      `{"tags":["work","personal"],"title":"holiday"}`,
			"tool.input.nested.tags": `[` + `"work"` + `,` + `"personal"` + `]`,
		} {
			if got, ok := point.When.Fields.ReadConditionField(field); !ok || got != want {
				t.Errorf("condition field %q = %q, %v; want %q", field, got, ok, want)
			}
			if got := point.Values[field]; got != want {
				t.Errorf("template value %q = %q, want %q", field, got, want)
			}
		}
	})
}

func TestEnricherMapsImplicitSkillAndPostOutput(t *testing.T) {
	event := runtimeEvent(agenthooks.KindToolPost, "PostToolUse", `{"session_id":"s4","tool_name":"Bash","tool_input":{"command":"cat /repo/.agents/skills/review/SKILL.md"},"tool_response":"manifest body"}`)
	tool := agenthooks.ToolCall{
		Name: "Bash", Canonical: agenthooks.ToolShell,
		Input:    json.RawMessage(`{"command":"cat /repo/.agents/skills/review/SKILL.md"}`),
		RawInput: json.RawMessage(`{"command":"cat /repo/.agents/skills/review/SKILL.md"}`),
	}
	post := &agenthooks.ToolPostEvent{Event: *event, Tool: tool, Output: json.RawMessage(`"manifest body"`)}
	input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "s4", AgentID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	point := input.Points[0]
	if point.SkillName != "review" || point.Values["skill.name"] != "review" || point.Values["tool.output"] != "manifest body" {
		t.Fatalf("implicit skill point = %#v", point)
	}
	if got, ok := point.When.Fields.ReadConditionField("skill.name"); !ok || got != "review" {
		t.Fatalf("skill condition field = %q, %v", got, ok)
	}
}

func TestEnricherDoesNotInferSkillFromAnUnknownReadTool(t *testing.T) {
	for _, name := range []string{"skills.read", "skills_read"} {
		t.Run(name, func(t *testing.T) {
			event := runtimeEvent(agenthooks.KindToolPost, "PostToolUse", `{}`)
			post := &agenthooks.ToolPostEvent{Event: *event, Tool: agenthooks.ToolCall{
				Name: name, Input: json.RawMessage(`{"name":"review","path":"skills/review/SKILL.md"}`),
			}, Output: json.RawMessage(`"manifest body"`)}
			input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "unknown-read", AgentID: "main"})
			if err != nil {
				t.Fatal(err)
			}
			if len(input.Points) != 1 || input.Points[0].SkillName != "" || input.Points[0].Values["skill.name"] != "" {
				t.Fatalf("unknown tool was inferred as an activated skill: %#v", input.Points)
			}
		})
	}
}

func TestEnricherProjectsSubagentScope(t *testing.T) {
	event := runtimeEvent(agenthooks.KindSubagentStart, "SubagentStart", `{"session_id":"s5","agent_id":"child-7","agent_type":"reviewer"}`)
	event.Agent = &agenthooks.AgentInfo{ID: "child-7", Type: "reviewer"}
	input, _, err := NewEnricher().Enrich(event, &agenthooks.SubagentStartEvent{Event: *event}, state.Session{HarnessVersion: "0.146.0"}, state.Scope{
		SessionID: "s5", AgentID: "child-7", PromptCount: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Scope.AgentID != "child-7" || input.TurnID != "9" {
		t.Fatalf("subagent envelope = %#v", input)
	}
	point := input.Points[0]
	if point.Name != "subagent.start" || point.When.AgentID != "child-7" || point.When.AgentType != "reviewer" || point.When.Harness["agent"] != "reviewer" {
		t.Fatalf("subagent point = %#v", point)
	}
	if !point.Capabilities.AddContext || point.Capabilities.Deny || point.Capabilities.SystemMessage {
		t.Fatalf("Codex SubagentStart capabilities = %#v", point.Capabilities)
	}
}

func TestEnricherRejectsMismatchedEventsAndProviders(t *testing.T) {
	enricher := NewEnricher()
	event := runtimeEvent(agenthooks.KindPromptSubmitted, "UserPromptSubmit", `{}`)
	if _, _, err := enricher.Enrich(event, &agenthooks.ToolPreEvent{Event: *event}, state.Session{}, state.Scope{}); err == nil {
		t.Fatal("expected a typed-event mismatch error")
	}
	event.Provider = agenthooks.ProviderClaudeCode
	if _, _, err := enricher.Enrich(event, event, state.Session{}, state.Scope{}); err == nil {
		t.Fatal("expected a wrong-provider error")
	}
}

func TestEnricherTrimsShellOutputTrailingWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canonical agenthooks.CanonicalTool
		response  string
		want      string
	}{
		// Codex 0.159.3 keeps the trailing newline; Claude Code trims it.
		{"Bash", agenthooks.ToolShell, `"  lead\nmid\n\n\n  \n"`, "  lead\nmid"},
		{"Bash", agenthooks.ToolShell, `"err-line\nout-line\n"`, "err-line\nout-line"},
		{"mcp__docs__search", agenthooks.ToolMCP, `"result\n"`, "result\n"},
	} {
		event := runtimeEvent(agenthooks.KindToolPost, "PostToolUse", `{"session_id":"s10","tool_name":"`+tc.name+`","tool_input":{"command":"x"},"tool_response":`+tc.response+`}`)
		tool := agenthooks.ToolCall{Name: tc.name, Canonical: tc.canonical, Input: json.RawMessage(`{"command":"x"}`)}
		post := &agenthooks.ToolPostEvent{Event: *event, Tool: tool, Output: json.RawMessage(tc.response)}
		input, _, err := NewEnricher().Enrich(event, post, state.Session{}, state.Scope{SessionID: "s10", AgentID: "main"})
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := input.Points[0].When.Fields.ReadConditionField("tool.output"); !ok || got != tc.want {
			t.Errorf("%s tool.output for %s = %q, %v; want %q", tc.name, tc.response, got, ok, tc.want)
		}
	}
}

func TestEnrichKeepsShellCharactersInToolInput(t *testing.T) {
	raw := `{"command":"cd repo && git push > out.log 2<&1","nested":{"q":"a&b<c>"}}`
	event := runtimeEvent(agenthooks.KindToolPre, "PreToolUse", `{"session_id":"s9","tool_name":"Bash","tool_input":`+raw+`}`)
	pre := &agenthooks.ToolPreEvent{Event: *event, Tool: agenthooks.ToolCall{Name: "Bash", Canonical: agenthooks.ToolShell, Input: json.RawMessage(raw)}}
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

func runtimeEvent(kind agenthooks.EventKind, nativeName, raw string) *agenthooks.Event {
	var session struct {
		ID             string `json:"session_id"`
		TurnID         string `json:"turn_id"`
		CWD            string `json:"cwd"`
		Model          string `json:"model"`
		PermissionMode string `json:"permission_mode"`
	}
	_ = json.Unmarshal([]byte(raw), &session)
	return &agenthooks.Event{
		Provider: agenthooks.ProviderCodex, NativeName: nativeName, Kind: kind,
		Session: agenthooks.SessionInfo{
			ID: session.ID, TurnID: session.TurnID, CWD: session.CWD,
			Model: session.Model, PermissionMode: session.PermissionMode,
		},
		Raw: json.RawMessage(raw),
	}
}

func TestFlattenJSONFieldsSkipsDottedAndEmptyKeys(t *testing.T) {
	raw := json.RawMessage(`{"a.b":"dot","a":{"b":"nested"},"":"empty","x":{"c.d":{"e":"deep"}}}`)
	want := map[string]string{
		"p":     `{"":"empty","a":{"b":"nested"},"a.b":"dot","x":{"c.d":{"e":"deep"}}}`,
		"p.a":   `{"b":"nested"}`,
		"p.a.b": "nested",
		"p.x":   `{"c.d":{"e":"deep"}}`,
	}
	// Map iteration order varies between runs; the result must not.
	for range 50 {
		got := map[string]string{}
		flattenJSONFields(got, "p", raw)
		if len(got) != len(want) {
			t.Fatalf("fields = %#v", got)
		}
		for key, value := range want {
			if got[key] != value {
				t.Fatalf("%s = %q, want %q (fields %#v)", key, got[key], value, got)
			}
		}
	}
}
