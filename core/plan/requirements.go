package plan

import (
	"fmt"
	"strings"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/template"
	stylewhen "github.com/inxups/style-cli/core/when"
)

const (
	SessionStart    = "session.start"
	PromptSubmitted = "prompt.submitted"
	SkillExplicit   = "skill.explicit"
	ToolPre         = "tool.pre"
	ToolPost        = "tool.post"
	ToolError       = "tool.error"
	SubagentStart   = "subagent.start"
)

// RequirementFor derives adapter-independent needs from one merged rule.
// Callers should validate the package before planning.
func RequirementFor(rule spec.Rule) (Requirement, error) {
	effective := rule.Effective
	if effective == nil {
		return Requirement{}, fmt.Errorf("rule %q has not been merged", rule.Source)
	}
	requirement := Requirement{
		RuleID:   firstString(effective["id"], rule.Fields["id"], rule.Source),
		Target:   stringValue(effective["target"]),
		Action:   stringValue(effective["action"]),
		Fallback: stringValue(effective["fallback"]),
		Mode:     stringValue(effective["mode"]),
		Repeat:   stringValue(effective["repeat"]),
	}
	if requirement.Target == "native" {
		// Native rules bypass the rule engine: no action, data or events.
		requirement.Action, requirement.Mode, requirement.Repeat = "", "", ""
		if requirement.Fallback == "" {
			requirement.Fallback = "degrade"
		}
		return requirement, nil
	}
	requirement.Refresh, _ = spec.RefreshInterval(effective["refresh"])
	if requirement.Action == "" {
		requirement.Action = "inject"
	}
	if requirement.Fallback == "" {
		requirement.Fallback = "degrade"
	}
	if requirement.Mode == "" {
		requirement.Mode = "append"
	}
	if requirement.Repeat == "" {
		requirement.Repeat = "session"
		if requirement.Action == "warn" {
			requirement.Repeat = "every"
		}
	}
	requirement.StaticPrefix = staticFallbackPrefix(effective)

	when := stringMap(effective["when"])
	requirement.HasWhen = len(when) > 0
	if _, exists := when["provider"]; exists {
		requirement.ProviderCondition = true
		requirement.Needs = append(requirement.Needs, "provider")
		if stylewhen.AllowsUnknown("provider", when["provider"]) {
			requirement.UnknownNeeds = append(requirement.UnknownNeeds, "provider")
		} else {
			requirement.ConditionNeeds = append(requirement.ConditionNeeds, "provider")
		}
	}
	requirement.Needs = append(requirement.Needs, "agent")
	requirement.ConditionNeeds = append(requirement.ConditionNeeds, "agent")
	if value, exists := when["model"]; exists {
		if value != nil {
			requirement.Needs = append(requirement.Needs, "model")
			if stylewhen.AllowsUnknown("model", when["model"]) {
				requirement.UnknownNeeds = append(requirement.UnknownNeeds, "model")
			} else {
				requirement.ConditionNeeds = append(requirement.ConditionNeeds, "model")
			}
		}
	}
	if _, exists := when["thinking_effort"]; exists {
		requirement.Needs = append(requirement.Needs, "thinking_effort")
		if stylewhen.AllowsUnknown("thinking_effort", when["thinking_effort"]) {
			requirement.UnknownNeeds = append(requirement.UnknownNeeds, "thinking_effort")
		} else {
			requirement.ConditionNeeds = append(requirement.ConditionNeeds, "thinking_effort")
		}
	}
	if _, exists := when["thinking_effort_approx"]; exists {
		requirement.Needs = append(requirement.Needs, "thinking_effort_approx")
		requirement.ConditionNeeds = append(requirement.ConditionNeeds, "thinking_effort_approx")
	}
	if _, exists := when["agent"]; exists {
		requirement.Needs = append(requirement.Needs, "agent")
		requirement.ConditionNeeds = append(requirement.ConditionNeeds, "agent")
	}
	for key := range stringMap(when["harness"]) {
		requirement.Needs = append(requirement.Needs, "harness."+key)
		if stylewhen.AllowsUnknown("harness."+key, stringMap(when["harness"])[key]) {
			requirement.UnknownNeeds = append(requirement.UnknownNeeds, "harness."+key)
		} else {
			requirement.ConditionNeeds = append(requirement.ConditionNeeds, "harness."+key)
		}
	}
	if conditions, ok := when["conditions"].([]any); ok {
		for _, condition := range conditions {
			if field := stringValue(stringMap(condition)["field"]); field != "" {
				requirement.ConditionNeeds = append(requirement.ConditionNeeds, field)
				requirement.Needs = append(requirement.Needs, field)
			}
		}
	}

	keys := []string{"text"}
	if requirement.Action == "warn" {
		keys = append(keys, "model_text")
	}
	for _, key := range keys {
		value := effective[key]
		if key == "text" {
			value = rule.Body
		}
		text, ok := value.(string)
		if !ok || text == "" {
			continue
		}
		parsed, err := template.Parse(text)
		if err != nil {
			return Requirement{}, fmt.Errorf("rule %q %s: %w", requirement.RuleID, key, err)
		}
		if key == "text" {
			requirement.StaticBody = parsed.IsStatic()
		}
		for _, part := range parsed.Parts() {
			if part.Kind == template.VariablePart && template.IsRuntimeVariable(part.Name) {
				requirement.Needs = append(requirement.Needs, part.Name)
			}
		}
	}

	requirement.Needs = sortedUnique(requirement.Needs)
	requirement.ConditionNeeds = sortedUnique(requirement.ConditionNeeds)
	requirement.AllCapabilities, requirement.AnyCapabilities = capabilitiesFor(requirement.Action)
	if requirement.Target == "system" {
		if requirement.Action == "inject" && requirement.StaticBody && !requirement.HasWhen && requirement.Refresh == 0 {
			requirement.Events = nil
		} else {
			requirement.Events = []string{SessionStart, PromptSubmitted}
		}
	} else {
		initial, alternate := eventFor(effective)
		switch requirement.Target {
		case "tool", "mcp":
			requirement.Events = appendEvent(initial)
			if alternate != "" && requirement.Action != "block" {
				requirement.Events = append(requirement.Events, alternate)
			}
		case "skill":
			requirement.Events = appendEvent(initial)
			if alternate != "" && requirement.Action != "block" {
				requirement.Events = append(requirement.Events, alternate)
			}
			requirement.Events = append(requirement.Events, SkillExplicit)
		case "event":
			requirement.Events = appendEvent(stringValue(effective["event"]))
		case "native":
		default:
			return Requirement{}, fmt.Errorf("rule %q has unknown target %q", requirement.RuleID, requirement.Target)
		}
	}
	return requirement, nil
}

func staticFallbackPrefix(fields map[string]any) string {
	var kind string
	var selectors []string
	switch stringValue(fields["target"]) {
	case "tool":
		kind = "When using "
		selectors = valuesAsStrings(fields["tool"])
		selectors = append(selectors, valuesAsStrings(fields["names"])...)
	case "mcp":
		kind = "When using MCP "
		selectors = valuesAsStrings(fields["mcp"])
	case "skill":
		kind = "When using skill "
		selectors = valuesAsStrings(fields["skill"])
	default:
		return ""
	}
	if len(selectors) == 0 {
		return ""
	}
	return kind + strings.Join(selectors, ", ") + ": "
}

func capabilitiesFor(action string) ([]Capability, []Capability) {
	switch action {
	case "warn":
		return nil, []Capability{AddContext, SystemMessage}
	case "block":
		return []Capability{Deny}, nil
	default:
		return []Capability{AddContext}, nil
	}
}

func eventFor(fields map[string]any) (initial, alternate string) {
	target := stringValue(fields["target"])
	switch target {
	case "tool", "mcp", "skill":
		at := stringValue(fields["at"])
		if at == "" {
			at = "after"
			if stringValue(fields["action"]) == "block" {
				at = "before"
			}
		}
		if at == "before" {
			return ToolPre, ToolPost
		}
		return ToolPost, ToolPre
	}
	return "", ""
}

func appendEvent(event string) []string {
	if event == "" {
		return nil
	}
	return []string{event}
}
