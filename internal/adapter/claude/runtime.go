package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/effortmap"
	"github.com/speakeasy-api/agenthooks"
)

const claudeRuntimeInputLimit = 10000

var _ adapter.RuntimeEnricher = (*Enricher)(nil)

// Enricher adapts Claude Code hook events to provider-neutral engine facts.
// Claude exposes a per-event effort level, followed by CLAUDE_EFFORT and the
// persisted session value. The optional Environment callback is injectable for
// tests and embedders; a nil callback reads CLAUDE_EFFORT from the process.
type Enricher struct {
	effortPolicy        effortmap.Policy
	providerEnabled     bool
	ProviderEnvironment func(string) (string, bool)
	OfflineProvider     *string
	ProviderOffline     bool
	HarnessVersion      string
	Environment         func() string
	InputLimit          tokens.InputLimit
}

// NewEnricher constructs a Claude runtime adapter with Claude's documented
// single-field input limit.
func NewEnricher() *Enricher {
	return &Enricher{InputLimit: tokens.InputLimit{Value: claudeRuntimeInputLimit, Unit: tokens.CharsUnit}}
}

// StateUpdate projects session facts and resolves effort in the documented
// event > environment > session order. A later event carrying an effort level
// remains authoritative even when an earlier hook already resolved the session.
func (enricher *Enricher) StateUpdate(event *agenthooks.Event, typed any, previous state.Session) (update state.EventUpdate) {
	if event == nil || event.Provider != agenthooks.ProviderClaudeCode {
		return update
	}
	defer func() {
		if enricher != nil && enricher.effortPolicy.Active(previous) {
			table, err := EffortTable()
			if err == nil {
				update = enricher.effortPolicy.Apply(update, previous, table)
			}
		}
	}()
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

	enricher.updateProvider(&update, event, previous)
	eventEffort := event.Session.EffortRaw
	if eventEffort == "" {
		eventEffort = rawString(event.RawField("effort.level"))
	}
	// Unlike Codex's once-per-session config lookup, Claude resolves on every
	// event: CLAUDE_EFFORT is consulted whenever the event lacks effort.level,
	// so a mid-session change reaches dynamic system rules (spec §6, §3.7).
	environment := ""
	if enricher != nil && enricher.Environment != nil {
		environment = enricher.Environment()
	} else {
		environment = os.Getenv("CLAUDE_EFFORT")
	}
	resolution, err := ResolveEffort(eventEffort, environment, previous)
	if err != nil {
		return update
	}
	update.ThinkingEffort = resolution.Effort
	update.EffortRaw = resolution.EffortRaw
	update.EffortApprox = cloneBool(resolution.EffortApprox)
	update.EffortResolved = true
	return update
}

// Enrich builds points, fields, scope, and runtime variables for one Claude
// hook invocation.
func (enricher *Enricher) Enrich(event *agenthooks.Event, typed any, session state.Session, scope state.Scope) (engine.Input, tokens.InputLimit, error) {
	if event == nil {
		return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: event is required")
	}
	if event.Provider != agenthooks.ProviderClaudeCode {
		return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: expected Claude Code event, got %q", event.Provider)
	}
	if scope.SessionID == "" {
		scope.SessionID = event.Session.ID
	}
	if scope.AgentID == "" {
		scope.AgentID = claudeAgentID(event)
	}
	if scope.AgentID == "" {
		scope.AgentID = "main"
	}

	variables := sessionVariables(event, session, scope, enricher)
	input := engine.Input{
		Harness:   string(agenthooks.ProviderClaudeCode),
		TurnID:    effectiveTurnID(event, scope),
		Scope:     scope,
		Variables: variables,
	}
	if start, ok := typed.(*agenthooks.SessionStartEvent); ok {
		input.Source = start.Source
	}
	limit := tokens.InputLimit{Value: claudeRuntimeInputLimit, Unit: tokens.CharsUnit}
	if enricher != nil && enricher.InputLimit.Value > 0 {
		limit = enricher.InputLimit
	}

	switch event.Kind {
	case agenthooks.KindSessionStart:
		if _, ok := typed.(*agenthooks.SessionStartEvent); !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: session.start has typed event %T", typed)
		}
		input.Points = []engine.Point{newPoint(event, plan.SessionStart, variables, nil, nil, "")}
	case agenthooks.KindPromptSubmitted:
		prompt, ok := typed.(*agenthooks.PromptEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: prompt.submitted has typed event %T", typed)
		}
		fields := map[string]string{}
		if prompt.Prompt != "" || hasRawField(event, "prompt") {
			fields["prompt"] = prompt.Prompt
		}
		input.Points = []engine.Point{newPoint(event, plan.PromptSubmitted, variables, fields, nil, "")}
	case agenthooks.KindPromptExpansion:
		expansion, ok := typed.(*agenthooks.PromptExpansionEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: prompt.expansion has typed event %T", typed)
		}
		// Claude's explicit skill event is the slash-command expansion. Empty
		// expansion_type is accepted for older payloads that omitted the field.
		if expansion.ExpansionType == "" || strings.EqualFold(expansion.ExpansionType, "slash_command") {
			fields := map[string]string{}
			if expansion.Prompt != "" || hasRawField(event, "prompt") {
				fields["prompt"] = expansion.Prompt
			}
			if expansion.CommandName != "" || hasRawField(event, "command_name") {
				fields["skill.name"] = expansion.CommandName
			}
			input.Points = []engine.Point{newPoint(event, plan.SkillExplicit, variables, fields, nil, expansion.CommandName)}
		}
	case agenthooks.KindToolPre:
		toolEvent, ok := typed.(*agenthooks.ToolPreEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: tool.pre has typed event %T", typed)
		}
		input.Points = []engine.Point{claudeToolPoint(event, plan.ToolPre, &toolEvent.Tool, nil, variables, typed)}
	case agenthooks.KindPermission:
		permission, ok := typed.(*agenthooks.PermissionEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: permission has typed event %T", typed)
		}
		// PermissionRequest carries the same tool input as PreToolUse. It is
		// projected onto tool.pre so existing tool rules can match it.
		input.Points = []engine.Point{claudeToolPoint(event, plan.ToolPre, &permission.Tool, nil, variables, typed)}
	case agenthooks.KindToolPost:
		toolEvent, ok := typed.(*agenthooks.ToolPostEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: tool.post has typed event %T", typed)
		}
		input.Points = []engine.Point{claudeToolPoint(event, plan.ToolPost, &toolEvent.Tool, toolEvent, variables, typed)}
	case agenthooks.KindToolError:
		toolEvent, ok := typed.(*agenthooks.ToolPostEvent)
		if !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: tool.error has typed event %T", typed)
		}
		input.Points = []engine.Point{claudeToolPoint(event, plan.ToolError, &toolEvent.Tool, toolEvent, variables, typed)}
	case agenthooks.KindSubagentStart:
		if _, ok := typed.(*agenthooks.SubagentStartEvent); !ok {
			return engine.Input{}, tokens.InputLimit{}, fmt.Errorf("claude adapter: subagent.start has typed event %T", typed)
		}
		input.Points = []engine.Point{newPoint(event, plan.SubagentStart, variables, nil, nil, "")}
	}
	for i := range input.Points {
		if enricher != nil && enricher.providerEnabled {
			input.Points[i].When.Provider = session.ModelProvider
			input.Points[i].When.ProviderSource = session.ProviderSource
		}
		if input.Points[i].When.AgentID == "" && scope.AgentID != "main" {
			input.Points[i].When.AgentID = scope.AgentID
		}
	}
	return input, limit, nil
}

func sessionVariables(event *agenthooks.Event, session state.Session, scope state.Scope, enricher *Enricher) map[string]string {
	variables := make(map[string]string, 16)
	if enricher != nil && enricher.providerEnabled && session.ModelProvider != "" {
		variables["provider"] = session.ModelProvider
	}
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
	variables["harness.name"] = string(agenthooks.ProviderClaudeCode)
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
	if event.Agent != nil && event.Agent.Type != "" {
		variables["harness.agent"] = event.Agent.Type
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
	conditionValues := cloneValues(values)
	point := engine.Point{
		Name:      name,
		SkillName: skillName,
		When:      claudeWhenInput(event, variables, conditionValues),
		Values:    values,
		Capabilities: engine.Capabilities{
			AddContext:             event.Can(agenthooks.CapAddContext),
			SystemMessage:          event.Can(agenthooks.CapSystemMessage),
			Deny:                   event.Can(agenthooks.CapDeny),
			SuppressOriginalPrompt: event.Kind == agenthooks.KindPromptSubmitted && event.Can(agenthooks.CapDeny),
		},
	}
	if tool != nil {
		point.ToolName = tool.Name
		canonical := tool.Canonical
		if canonical == "" && tool.Name != "" {
			canonical = agenthooks.CanonicalToolFor(tool.Name)
		}
		point.CanonicalTool = string(canonical)
		if mcp := claudeToolMCP(tool); mcp != nil {
			point.MCPServer, point.MCPTool = mcp.Server, mcp.Tool
		}
	}
	return point
}

func claudeToolPoint(event *agenthooks.Event, pointName string, tool *agenthooks.ToolCall, post *agenthooks.ToolPostEvent, variables map[string]string, typed any) engine.Point {
	fields := map[string]string{}
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
		input := tool.Input
		if len(input) == 0 {
			input = tool.RawInput
		}
		if len(input) > 0 && !bytes.Equal(bytes.TrimSpace(input), []byte("{}")) {
			if compact := compactJSON(input); compact != "" {
				fields["tool.input"] = compact
				flattenJSONFields(fields, "tool.input", input)
			}
		}
		if added, ok := claudeAdded(tool); ok {
			fields["tool.added"] = added
		}
		if mcp := claudeToolMCP(tool); mcp != nil {
			if mcp.Server != "" {
				fields["mcp.server"] = mcp.Server
			}
			if mcp.Tool != "" {
				fields["mcp.tool"] = mcp.Tool
			}
		}
	}
	if post != nil {
		if len(post.Output) > 0 {
			if output, available := rawFieldValue(post.Output); available {
				fields["tool.output"] = output
			}
		}
		if post.Error != "" && fields["tool.output"] == "" {
			fields["tool.output"] = post.Error
		}
	}
	if event.Session.CWD != "" || hasRawField(event, "cwd") {
		fields["cwd"] = event.Session.CWD
	}
	name := ""
	if activation := agenthooks.SkillActivationOf(typed); activation != nil {
		name = activation.Name
		fields["skill.name"] = name
	}
	return newPoint(event, pointName, variables, fields, tool, name)
}

func claudeAdded(tool *agenthooks.ToolCall) (string, bool) {
	if tool == nil {
		return "", false
	}
	var key string
	switch strings.ToLower(tool.Name) {
	case "edit":
		key = "new_string"
	case "write":
		key = "content"
	default:
		return "", false
	}
	input := tool.Input
	if len(input) == 0 {
		input = tool.RawInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return "", false
	}
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	value, available := rawFieldValue(raw)
	return value, available
}

func claudeToolMCP(tool *agenthooks.ToolCall) *agenthooks.MCPCall {
	if tool == nil {
		return nil
	}
	if tool.MCP != nil {
		return tool.MCP
	}
	return agenthooks.ParseMCPName(tool.Name)
}

func claudeWhenInput(event *agenthooks.Event, variables, fields map[string]string) when.Input {
	harness := map[string]string{}
	for name, value := range variables {
		if strings.HasPrefix(name, "harness.") {
			harness[strings.TrimPrefix(name, "harness.")] = value
		}
	}
	result := when.Input{
		Model:                variables["model"],
		ThinkingEffort:       variables["thinking_effort"],
		Harness:              harness,
		Fields:               claudeFields{event: event, values: cloneValues(fields)},
		ThinkingEffortApprox: approximation(variables["thinking_effort_approx"]),
	}
	if event.Agent != nil {
		result.AgentID = event.Agent.ID
		result.AgentType = event.Agent.Type
	}
	return result
}

type claudeFields struct {
	event  *agenthooks.Event
	values map[string]string
}

func (fields claudeFields) ReadConditionField(name string) (string, bool) {
	if value, ok := fields.values[name]; ok {
		return value, true
	}
	if strings.HasPrefix(name, "tool.input.") && fields.event != nil {
		path := "tool_input." + strings.TrimPrefix(name, "tool.input.")
		if raw := fields.event.RawField(path); len(raw) > 0 {
			return rawFieldValue(raw)
		}
	}
	return "", false
}

func claudeAgentID(event *agenthooks.Event) string {
	if event != nil && event.Agent != nil {
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

func commonFields(event *agenthooks.Event) map[string]string {
	fields := make(map[string]string, 1)
	if event != nil && (event.Session.CWD != "" || hasRawField(event, "cwd")) {
		fields["cwd"] = event.Session.CWD
	}
	return fields
}

// ConfigureEffortMapping binds the compiled snapshot before processing events.
func (enricher *Enricher) ConfigureEffortMapping(mapping compile.EffortMapping) {
	enricher.effortPolicy = effortmap.NewPolicy("claude-code", mapping)
}
