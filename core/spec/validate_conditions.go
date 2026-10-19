package spec

import (
	"fmt"
	"regexp"
	"strings"

	stylewhen "github.com/inxups/style-cli/core/when"
)

var effortLevels = stringSet("none", "minimal", "low", "medium", "high", "xhigh", "max")
var conditionOperators = stringSet("regex", "shell_regex", "contains", "equals", "not_contains", "starts_with", "ends_with")

// Substring operators are constant for an empty pattern: contains, starts_with
// and ends_with always match while not_contains never does.
var emptyPatternOperators = stringSet("contains", "not_contains", "starts_with", "ends_with")
var validEvents = stringSet("session.start", "prompt.submitted", "tool.pre", "tool.post", "tool.error", "subagent.start")

func (v *validator) validateWhen(value any, rule Rule, source string) {
	when := asStringMap(value)
	if when == nil {
		v.add("invalid-schema", source, "when must be a mapping")
		return
	}
	for _, key := range sortedMapKeys(when) {
		item := when[key]
		switch key {
		case "model":
			if !isStringOrStringList(item) {
				v.add("invalid-schema", source, "when.model must be a string or list of strings")
			} else {
				for _, pattern := range stringValues(item) {
					if err := stylewhen.ValidateModelGlob(pattern); err != nil {
						v.add("bad-model-glob", source, "when.model contains an invalid glob pattern")
					}
				}
			}
		case "thinking_effort":
			if !validEffortExpression(item) {
				v.add("bad-effort-expr", source, "thinking_effort must be a tier, list of tiers, or ordered comparison")
			}
		case "thinking_effort_approx":
			if _, ok := item.(bool); !ok {
				v.add("invalid-schema", source, "when.thinking_effort_approx must be a boolean")
			}
		case "harness":
			v.validateHarnessWhen(item, source)
		case "agent":
			if !validAgentExpression(item) {
				v.add("invalid-schema", source, "when.agent must be any or a string/list of agent names")
			}
		case "conditions":
			v.validateConditions(item, rule, source)
		case "provider":
			if !isStringOrStringList(item) {
				v.add("invalid-schema", source, "when.provider must be a string or list of strings")
			} else {
				for _, id := range stringValues(item) {
					if id != "unknown" && !stylewhen.ValidProviderID(id) {
						v.add("bad-provider-id", source, "when.provider requires bounded service identifiers")
					}
				}
			}
		default:
			v.add("invalid-schema", source, fmt.Sprintf("unknown when field %q", key))
		}
	}
}

func (v *validator) validateHarnessWhen(value any, source string) {
	harness := asStringMap(value)
	if harness == nil {
		v.add("invalid-schema", source, "when.harness must be a mapping")
		return
	}
	for _, field := range sortedMapKeys(harness) {
		if field != "name" && field != "permission_mode" {
			v.add("invalid-schema", source, fmt.Sprintf("unknown when.harness field %q", field))
			continue
		}
		if !isStringOrStringList(harness[field]) {
			v.add("invalid-schema", source, fmt.Sprintf("when.harness.%s must be a string or list of strings", field))
		} else if field == "name" {
			for _, name := range stringValues(harness[field]) {
				if v.harnesses != nil && !v.harnesses[name] {
					v.add("invalid-schema", source, fmt.Sprintf("unknown harness name %q", name))
				}
			}
		}
	}
}

func validEffortExpression(value any) bool {
	if text, ok := value.(string); ok {
		text = strings.ToLower(strings.TrimSpace(text))
		if len(text) > 2 && (strings.HasPrefix(text, ">=") || strings.HasPrefix(text, "<=")) {
			level := strings.TrimSpace(text[2:])
			return effortLevels[level]
		}
		return effortLevels[text] || text == "open" || text == "unknown"
	}
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, value := range values {
		level, ok := value.(string)
		if !ok {
			return false
		}
		level = strings.ToLower(strings.TrimSpace(level))
		if !effortLevels[level] && level != "open" && level != "unknown" {
			return false
		}
	}
	return true
}

func validAgentExpression(value any) bool {
	if text, ok := value.(string); ok {
		return text == "any" || text != ""
	}
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, value := range values {
		if text, ok := value.(string); !ok || text == "" {
			return false
		}
	}
	return true
}

func (v *validator) validateConditions(value any, rule Rule, source string) {
	conditions, ok := value.([]any)
	if !ok {
		v.add("invalid-schema", source, "when.conditions must be a list")
		return
	}
	for index, entry := range conditions {
		conditionSource := fmt.Sprintf("%s:when.conditions[%d]", source, index)
		fields := asStringMap(entry)
		if fields == nil {
			v.add("invalid-schema", conditionSource, "condition must be a mapping")
			continue
		}
		for _, key := range sortedMapKeys(fields) {
			if key != "field" && key != "op" && key != "pattern" {
				v.add("invalid-schema", conditionSource, fmt.Sprintf("unknown condition field %q", key))
			}
		}
		field, fieldOK := fields["field"].(string)
		op, opOK := fields["op"].(string)
		pattern, patternOK := fields["pattern"].(string)
		if !fieldOK || !opOK || !patternOK {
			v.add("invalid-schema", conditionSource, "condition requires string field, op, and pattern")
			continue
		}
		if !conditionOperators[op] {
			v.add("invalid-schema", conditionSource, fmt.Sprintf("unknown condition op %q", op))
		}
		if pattern == "" && emptyPatternOperators[op] {
			v.add("empty-pattern", conditionSource, fmt.Sprintf("%s requires a non-empty pattern; an empty pattern makes the condition constant", op))
		}
		if op == "regex" || op == "shell_regex" {
			if _, err := regexp.Compile(pattern); err != nil {
				v.add("bad-regex", conditionSource, "condition pattern is not a valid RE2 regular expression")
			}
		}
		if !conditionFieldKnown(field) {
			v.add("invalid-schema", conditionSource, fmt.Sprintf("unknown condition field %q", field))
		} else if !v.conditionFieldAvailable(field, rule) {
			v.add("condition-field-wrong-event", conditionSource, fmt.Sprintf("%s is unavailable on this rule's events", field))
		} else if op == "not_contains" && v.conditionFieldOptional(field, rule) {
			v.warn("negated-optional-field", conditionSource, fmt.Sprintf("%s may be unavailable; not_contains will not match when it is missing", field))
		}
		if field == "tool.input" {
			// The whole input is compact JSON: quotes, newlines and backslashes stay
			// escaped, key names are part of the text, and anchors see the braces.
			v.warn("whole-tool-input", conditionSource, "tool.input is the whole input as JSON text; match a decoded value such as tool.input.command instead")
		}
	}
}

func conditionFieldKnown(field string) bool {
	if stringSet("prompt", "tool.name", "tool.canonical", "tool.input", "tool.added", "tool.output", "mcp.server", "mcp.tool", "skill.name", "cwd")[field] {
		return true
	}
	return strings.HasPrefix(field, "tool.input.") && len(field) > len("tool.input.")
}

func (v *validator) conditionFieldAvailable(field string, rule Rule) bool {
	target := v.effectiveRuleString(rule, "target", "")
	event := v.effectiveRuleString(rule, "event", "")
	allowedEvents := map[string]bool{}
	switch target {
	case "system":
		allowedEvents["session.start"], allowedEvents["prompt.submitted"] = true, true
	case "tool", "mcp":
		fallbackAt := "after"
		if v.effectiveRuleString(rule, "action", "inject") == "block" {
			fallbackAt = "before"
		}
		if v.effectiveRuleString(rule, "at", fallbackAt) == "before" {
			allowedEvents["tool.pre"] = true
		} else {
			allowedEvents["tool.post"] = true
		}
	case "skill":
		fallbackAt := "after"
		if v.effectiveRuleString(rule, "action", "inject") == "block" {
			fallbackAt = "before"
		}
		if v.effectiveRuleString(rule, "at", fallbackAt) == "before" {
			allowedEvents["tool.pre"] = true
		} else {
			allowedEvents["tool.post"] = true
		}
		// Explicit skill invocation is a separate evaluation point and is
		// available regardless of the implicit tool timing.
		allowedEvents["skill.explicit"] = true
	case "event":
		allowedEvents[event] = true
	}
	if field == "cwd" {
		return true
	}
	if field == "prompt" {
		return allowedEvents["prompt.submitted"] || allowedEvents["skill.explicit"]
	}
	if field == "tool.output" {
		return allowedEvents["tool.post"]
	}
	if strings.HasPrefix(field, "mcp.") {
		return target == "mcp" || (target == "event" && strings.HasPrefix(event, "tool."))
	}
	if field == "skill.name" {
		return target == "skill" || (target == "event" && event == "skill.explicit")
	}
	if field == "tool.added" {
		if target == "tool" {
			// Native-name support is provider-specific. Accept the selector here
			// and let the adapter expose tool.added only when it can extract it;
			// the runtime matcher treats unavailable fields as non-matches.
			if len(v.effectiveRuleStringValues(rule, "names")) > 0 {
				return allowedEvents["tool.pre"] || allowedEvents["tool.post"]
			}
			for _, tool := range v.effectiveRuleStringValues(rule, "tool") {
				if tool == "file.edit" || tool == "file.write" {
					return allowedEvents["tool.pre"] || allowedEvents["tool.post"]
				}
			}
			return false
		}
		return target == "event" && (event == "tool.pre" || event == "tool.post")
	}
	if strings.HasPrefix(field, "tool.") {
		return allowedEvents["tool.pre"] || allowedEvents["tool.post"] || allowedEvents["tool.error"]
	}
	return false
}

func (v *validator) conditionFieldOptional(field string, rule Rule) bool {
	if field == "tool.added" || strings.HasPrefix(field, "tool.input.") {
		return true
	}
	target := v.effectiveRuleString(rule, "target", "")
	if field == "prompt" && (target == "system" || target == "skill") {
		return true
	}
	return strings.HasPrefix(field, "mcp.") && target == "event"
}
