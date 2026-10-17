package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks"
)

const codexRuntimeInputLimit = 2500

var _ adapter.RuntimeEnricher = (*Enricher)(nil)

// Enricher adapts Codex's decoded hook events to provider-neutral engine facts.
// Project trust defaults to false: callers must provide a positive trust
// decision before project configuration can affect effort matching.
type Enricher struct {
	Home string
	// OfflineConfigEffort replaces all external config and process discovery during replay.
	OfflineConfigEffort *string
	Offline             bool
	// HarnessVersion is the detected installation version. Event data takes
	// precedence when present.
	HarnessVersion string
	// ProjectTrusted reports whether the current working directory is trusted
	// by Codex. A nil callback means project configuration is never read.
	ProjectTrusted func(cwd string) bool
	// LaunchSettings may be supplied by callers/tests. A nil callback uses
	// agenthooks' process-ancestor reader.
	LaunchSettings func(cwd string) (agenthooks.CodexLaunchSettings, bool)
	ReadFile       func(string) ([]byte, error)
	InputLimit     tokens.InputLimit
}

// NewEnricher constructs a Codex runtime adapter, applying the documented
// native input limit when none is explicitly supplied.
func NewEnricher() *Enricher {
	return &Enricher{InputLimit: tokens.InputLimit{Value: codexRuntimeInputLimit, Unit: tokens.TokensUnit}}
}

// StateUpdate projects session facts and resolves configured effort once.
// Unknown/unavailable effort is still marked resolved by the existing
// effort resolver so hot-path hooks do not repeatedly read configuration.
func (enricher *Enricher) StateUpdate(event *agenthooks.Event, typed any, previous state.Session) state.EventUpdate {
	update := state.EventUpdate{}
	if event == nil || event.Provider != agenthooks.ProviderCodex {
		return update
	}
	update.Model = event.Session.Model
	update.HarnessVersion = event.Session.HarnessVersion
	if update.HarnessVersion == "" && enricher != nil {
		update.HarnessVersion = enricher.HarnessVersion
	}
	if event.Kind == agenthooks.KindSessionStart {
		if start, ok := typed.(*agenthooks.SessionStartEvent); ok {
			update.Source = start.Source
		}
		if update.Source == "" {
			update.Source = rawString(event.RawField("source"))
		}
	}
	if previous.EffortResolved {
		return update
	}

	if enricher != nil && enricher.Offline {
		update.EffortResolved = true
		if enricher.OfflineConfigEffort != nil {
			table, err := EffortTable()
			if err == nil {
				value := table.Resolve(*enricher.OfflineConfigEffort)
				update.ThinkingEffort = value.Effort
				update.EffortRaw = value.EffortRaw
				update.EffortApprox = cloneBool(value.EffortApprox)
			}
		}
		return update
	}
	input := EffortInput{CWD: event.Session.CWD, HarnessVersion: update.HarnessVersion}
	if enricher != nil {
		input.Home = enricher.Home
		input.ReadFile = enricher.ReadFile
		if enricher.ProjectTrusted != nil {
			input.ProjectTrusted = enricher.ProjectTrusted(event.Session.CWD)
		}
		if enricher.LaunchSettings != nil {
			input.Launch, _ = enricher.LaunchSettings(event.Session.CWD)
		} else {
			input.Launch, _ = agenthooks.CurrentCodexLaunchSettings(event.Session.CWD)
		}
	} else {
		input.Launch, _ = agenthooks.CurrentCodexLaunchSettings(event.Session.CWD)
	}
	effort := StateUpdate(input, previous)
	update.ThinkingEffort = effort.ThinkingEffort
	update.EffortRaw = effort.EffortRaw
	update.EffortApprox = effort.EffortApprox
	update.EffortResolved = effort.EffortResolved
	return update
}

// Enrich builds the points and values available to the Codex runtime engine.
func (enricher *Enricher) Enrich(event *agenthooks.Event, typed any, session state.Session, scope state.Scope) (engine.Input, tokens.InputLimit, error) {
	if event == nil {
		return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: event is required")
	}
	if event.Provider != agenthooks.ProviderCodex {
		return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: expected Codex event, got %q", event.Provider)
	}
	if scope.SessionID == "" {
		scope.SessionID = event.Session.ID
	}
	if scope.AgentID == "" {
		scope.AgentID = agentID(event)
	}
	if scope.AgentID == "" {
		scope.AgentID = "main"
	}

	variables := sessionVariables(event, session, scope, enricher)
	input := engine.Input{
		Harness:   string(agenthooks.ProviderCodex),
		TurnID:    effectiveTurnID(event, scope),
		Scope:     scope,
		Variables: variables,
	}
	limit := tokens.InputLimit{Value: codexRuntimeInputLimit, Unit: tokens.TokensUnit}
	if enricher != nil && enricher.InputLimit.Value > 0 {
		limit = enricher.InputLimit
	}

	switch event.Kind {
	case agenthooks.KindSessionStart:
		if _, ok := typed.(*agenthooks.SessionStartEvent); !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: session.start has typed event %T", typed)
		}
		input.Points = []engine.Point{newPoint(event, "session.start", variables, nil, nil, "")}
	case agenthooks.KindPromptSubmitted:
		prompt, ok := typed.(*agenthooks.PromptEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: prompt.submitted has typed event %T", typed)
		}
		fields := make(map[string]string)
		if hasRawField(event, "prompt") || prompt.Prompt != "" {
			fields["prompt"] = prompt.Prompt
		}
		input.Points = append(input.Points, newPoint(event, "prompt.submitted", variables, fields, nil, ""))
		for _, name := range ExplicitSkills(prompt.Prompt) {
			skillFields := cloneValues(fields)
			skillFields["skill.name"] = name
			input.Points = append(input.Points, newPoint(event, "skill.explicit", variables, skillFields, nil, name))
		}
	case agenthooks.KindToolPre:
		toolEvent, ok := typed.(*agenthooks.ToolPreEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: tool.pre has typed event %T", typed)
		}
		input.Points = []engine.Point{toolPoint(event, &toolEvent.Tool, nil, variables)}
	case agenthooks.KindPermission:
		permission, ok := typed.(*agenthooks.PermissionEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: permission has typed event %T", typed)
		}
		input.Points = []engine.Point{toolPoint(event, &permission.Tool, nil, variables)}
	case agenthooks.KindToolPost:
		toolEvent, ok := typed.(*agenthooks.ToolPostEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: tool.post has typed event %T", typed)
		}
		input.Points = []engine.Point{toolPoint(event, &toolEvent.Tool, toolEvent, variables)}
	case agenthooks.KindSubagentStart:
		if _, ok := typed.(*agenthooks.SubagentStartEvent); !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("codex adapter: subagent.start has typed event %T", typed)
		}
		input.Points = []engine.Point{newPoint(event, "subagent.start", variables, nil, nil, "")}
	}
	for i := range input.Points {
		if input.Points[i].When.AgentID == "" && scope.AgentID != "main" {
			input.Points[i].When.AgentID = scope.AgentID
		}
	}
	return input, limit, nil
}

func sessionVariables(event *agenthooks.Event, session state.Session, scope state.Scope, enricher *Enricher) map[string]string {
	variables := make(map[string]string, 12)
	model := session.Model
	if model == "" {
		model = event.Session.Model
	}
	if model != "" {
		variables["model"] = model
	}
	if session.ThinkingEffort != "" {
		variables["thinking_effort"] = session.ThinkingEffort
	}
	if session.EffortRaw != "" {
		variables["thinking_effort_raw"] = session.EffortRaw
	}
	if session.EffortApprox != nil {
		variables["thinking_effort_approx"] = strconv.FormatBool(*session.EffortApprox)
	}
	variables["harness.name"] = string(agenthooks.ProviderCodex)
	if event.Session.CWD != "" || hasRawField(event, "cwd") {
		variables["harness.cwd"] = event.Session.CWD
	}
	if event.Session.PermissionMode != "" || hasRawField(event, "permission_mode") {
		variables["harness.permission_mode"] = event.Session.PermissionMode
	}
	if event.Session.ID != "" || hasRawField(event, "session_id") {
		variables["harness.session_id"] = event.Session.ID
	}
	if turnID := effectiveTurnID(event, scope); turnID != "" {
		variables["harness.turn_id"] = turnID
	}
	version := session.HarnessVersion
	if version == "" {
		version = event.Session.HarnessVersion
	}
	if version == "" && enricher != nil {
		version = enricher.HarnessVersion
	}
	if version != "" {
		variables["harness.version"] = version
	}
	if event.Agent != nil {
		if event.Agent.Type != "" {
			variables["harness.agent"] = event.Agent.Type
		}
	} else if scope.AgentID == "" || scope.AgentID == "main" {
		variables["harness.agent"] = "main"
	}
	return variables
}

func effectiveTurnID(event *agenthooks.Event, scope state.Scope) string {
	if event.Session.TurnID != "" {
		return event.Session.TurnID
	}
	return strconv.FormatUint(scope.PromptCount, 10)
}

func newPoint(event *agenthooks.Event, name string, variables, fields map[string]string, tool *agenthooks.ToolCall, skillName string) engine.Point {
	values := commonFields(event)
	for key, value := range fields {
		values[key] = value
	}
	conditionValues := commonFields(event)
	for key, value := range fields {
		conditionValues[key] = value
	}
	point := engine.Point{
		Name:      name,
		SkillName: skillName,
		When:      whenInput(event, variables, conditionValues),
		Values:    values,
		Capabilities: engine.Capabilities{
			AddContext:    event.Can(agenthooks.CapAddContext),
			SystemMessage: event.Can(agenthooks.CapSystemMessage),
			Deny:          event.Can(agenthooks.CapDeny),
		},
	}
	if tool != nil {
		point.ToolName = tool.Name
		canonical := tool.Canonical
		if canonical == "" && tool.Name != "" {
			canonical = agenthooks.CanonicalToolFor(tool.Name)
		}
		point.CanonicalTool = string(canonical)
		if mcp := toolMCP(tool); mcp != nil {
			point.MCPServer, point.MCPTool = mcp.Server, mcp.Tool
		}
	}
	return point
}

func toolPoint(event *agenthooks.Event, tool *agenthooks.ToolCall, post *agenthooks.ToolPostEvent, variables map[string]string) engine.Point {
	fields := make(map[string]string)
	name := ""
	if tool != nil {
		if tool.Name != "" {
			fields["tool.name"] = tool.Name
		}
		canonical := tool.Canonical
		if canonical == "" && tool.Name != "" {
			canonical = agenthooks.CanonicalToolFor(tool.Name)
		}
		if canonical != "" {
			fields["tool.canonical"] = string(canonical)
		}
		if len(tool.RawInput) > 0 || len(tool.Input) > 0 && !bytes.Equal(bytes.TrimSpace(tool.Input), []byte("{}")) {
			input := compactJSON(tool.Input)
			if input != "" {
				fields["tool.input"] = input
				flattenJSONFields(fields, "tool.input", tool.Input)
			}
		}
		mcp := toolMCP(tool)
		if mcp != nil {
			if mcp.Server != "" {
				fields["mcp.server"] = mcp.Server
			}
			if mcp.Tool != "" {
				fields["mcp.tool"] = mcp.Tool
			}
		}
	}
	if post != nil && len(post.Output) > 0 {
		if output, available := rawFieldValue(post.Output); available {
			fields["tool.output"] = output
		}
	}
	if event.Session.CWD != "" || hasRawField(event, "cwd") {
		fields["cwd"] = event.Session.CWD
	}
	if activation := agenthooks.SkillActivationOf(toolEvent(event, tool, post)); activation != nil {
		name = activation.Name
		fields["skill.name"] = name
	}
	if event.Kind == agenthooks.KindToolPre || event.Kind == agenthooks.KindToolPost {
		if tool != nil && strings.EqualFold(tool.Name, "apply_patch") {
			if patch, available := patchText(tool.Input); available {
				fields["tool.added"] = addedPatchLines(patch)
			}
		}
	}
	pointName := string(event.Kind)
	if event.Kind == agenthooks.KindPermission {
		pointName = "tool.pre"
	}
	point := newPoint(event, pointName, variables, fields, tool, name)
	if point.Name == "" {
		point.Name = "tool.pre"
		if event.Kind == agenthooks.KindToolPost {
			point.Name = "tool.post"
		}
	}
	return point
}

func toolEvent(event *agenthooks.Event, tool *agenthooks.ToolCall, post *agenthooks.ToolPostEvent) any {
	if post != nil {
		return post
	}
	if tool == nil {
		return event
	}
	return &agenthooks.ToolPreEvent{Event: *event, Tool: *tool}
}

func toolMCP(tool *agenthooks.ToolCall) *agenthooks.MCPCall {
	if tool == nil {
		return nil
	}
	if tool.MCP != nil {
		return tool.MCP
	}
	return agenthooks.ParseMCPName(tool.Name)
}

func whenInput(event *agenthooks.Event, variables, fields map[string]string) when.Input {
	harness := make(map[string]string)
	for name, value := range variables {
		if strings.HasPrefix(name, "harness.") {
			harness[strings.TrimPrefix(name, "harness.")] = value
		}
	}
	result := when.Input{
		Model:                variables["model"],
		ThinkingEffort:       variables["thinking_effort"],
		Harness:              harness,
		Fields:               codexFields{event: event, values: cloneValues(fields)},
		ThinkingEffortApprox: approximation(variables["thinking_effort_approx"]),
	}
	if event.Agent != nil {
		result.AgentID = event.Agent.ID
		result.AgentType = event.Agent.Type
	}
	return result
}

func approximation(value string) *bool {
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil
	}
	return &parsed
}

type codexFields struct {
	event  *agenthooks.Event
	values map[string]string
}

func (fields codexFields) ReadConditionField(name string) (string, bool) {
	if value, ok := fields.values[name]; ok {
		return value, true
	}
	if strings.HasPrefix(name, "tool.input.") && fields.event != nil {
		path := "tool_input." + strings.TrimPrefix(name, "tool.input.")
		if raw := fields.event.RawField(path); len(raw) > 0 {
			return rawFieldValue(raw)
		}
	}
	value, ok := fields.values[name]
	return value, ok
}

func agentID(event *agenthooks.Event) string {
	if event.Agent != nil {
		return event.Agent.ID
	}
	return ""
}

func hasRawField(event *agenthooks.Event, path string) bool {
	if event == nil {
		return false
	}
	raw := bytes.TrimSpace(event.RawField(path))
	return len(raw) != 0 && !bytes.Equal(raw, []byte("null"))
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return ""
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return ""
	}
	return compact.String()
}

func rawFieldValue(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", false
	}
	var text string
	if json.Unmarshal(trimmed, &text) == nil {
		return text, true
	}
	return compactJSON(trimmed), true
}

func patchText(input json.RawMessage) (string, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return "", false
	}
	value, ok := fields["command"]
	if !ok {
		return "", false
	}
	var patch string
	if json.Unmarshal(value, &patch) != nil {
		return "", false
	}
	return patch, true
}

func addedPatchLines(patch string) string {
	lines := strings.Split(patch, "\n")
	added := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, strings.TrimPrefix(line, "+"))
		}
	}
	return strings.Join(added, "\n")
}

func commonFields(event *agenthooks.Event) map[string]string {
	fields := make(map[string]string, 1)
	if event != nil && (event.Session.CWD != "" || hasRawField(event, "cwd")) {
		fields["cwd"] = event.Session.CWD
	}
	return fields
}

func flattenJSONFields(destination map[string]string, prefix string, raw json.RawMessage) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return
	}
	var walk func(string, any)
	walk = func(path string, item any) {
		switch typed := item.(type) {
		case map[string]any:
			if encoded, ok := encodeJSONText(typed); ok {
				destination[path] = encoded
			}
			for key, child := range typed {
				walk(path+"."+key, child)
			}
		case []any:
			if encoded, ok := encodeJSONText(typed); ok {
				destination[path] = encoded
			}
			for index, child := range typed {
				walk(path+"."+strconv.Itoa(index), child)
			}
		case string:
			destination[path] = typed
		case json.Number:
			destination[path] = typed.String()
		case bool:
			destination[path] = strconv.FormatBool(typed)
		case nil:
		default:
			if encoded, ok := encodeJSONText(typed); ok {
				destination[path] = encoded
			}
		}
	}
	walk(prefix, value)
}

// encodeJSONText re-encodes a decoded tool input value without HTML escaping,
// so condition fields keep characters such as & < > as the harness sent them.
func encodeJSONText(value any) (string, bool) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(value) != nil {
		return "", false
	}
	return strings.TrimSuffix(encoded.String(), "\n"), true
}

func cloneValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
