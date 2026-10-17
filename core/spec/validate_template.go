package spec

import (
	"strings"

	styletemplate "github.com/inxups/style-cli/core/template"
)

var eventTemplateNames = []string{"prompt", "tool.name", "tool.input", "tool.added", "tool.output", "mcp.server", "mcp.tool", "skill.name"}

func (v *validator) validateTemplate(text, source string, rule Rule) {
	parsed, err := styletemplate.Parse(text)
	if err != nil {
		v.add("invalid-template", source, err.Error())
		return
	}
	for _, part := range parsed.Parts() {
		if part.Kind == styletemplate.VariablePart {
			v.validateTemplateName(part.Name, part.HasDefault, source, rule)
		}
	}
}

func (v *validator) validateTemplateName(name string, hasDefault bool, source string, rule Rule) {
	for _, eventName := range eventTemplateNames {
		if name == eventName || eventName == "tool.input" && strings.HasPrefix(name, "tool.input.") {
			v.add("event-var-in-template", source, name+" cannot be used in a template")
			return
		}
	}
	if strings.HasPrefix(name, "event.") {
		field := strings.TrimPrefix(name, "event.")
		if !styletemplate.EventFieldKnown(field) {
			v.add("undeclared-var", source, name+" is not a supported event variable")
			return
		}
		v.requireRuntimeRead(name, source)
		if !v.conditionFieldAvailable(field, rule) {
			v.add("event-var-unavailable", source, name+" is unavailable for this target/event")
		}
		if v.effectiveRuleString(rule, "action", "inject") == "warn" {
			if _, ok := effectiveRuleValue(rule, v.pkg, "model_text"); !ok {
				v.add("event-warn-model-text", source, "warn with event content requires explicit model_text to preserve complete fences")
			}
		}
		return
	}
	switch {
	case strings.HasPrefix(name, "vars."):
		vars := asStringMap(v.pkg.Metadata["vars"])
		if vars == nil || !hasKey(vars, strings.TrimPrefix(name, "vars.")) {
			v.add("undeclared-var", source, name+" is not declared")
		}
	case strings.HasPrefix(name, "settings."):
		if !v.settingDeclared(strings.TrimPrefix(name, "settings.")) {
			v.add("undeclared-var", source, name+" is not declared")
		}
	case name == "package.name", name == "package.version":
		key := strings.TrimPrefix(name, "package.")
		if _, exists := v.pkg.Metadata[key]; !exists {
			v.add("undeclared-var", source, name+" is not declared")
		}
	case name == "rule.id":
		if _, exists := rule.Fields["id"]; !exists {
			v.add("undeclared-var", source, name+" is not available")
		}
	case isRuntimeTemplateName(name):
		v.requireRuntimeRead(runtimeReadForTemplate(name), source)
		if strings.HasPrefix(name, "harness.") && name != "harness.session_id" && name != "harness.turn_id" && !harnessReadFields[strings.TrimPrefix(name, "harness.")] {
			v.add("undeclared-var", source, name+" is not a supported runtime variable")
		}
		if !hasDefault && !v.whenGuarantees(rule, name) {
			v.warn("var-no-default", source, name+" may be empty when unavailable; add a default or a matching when condition")
		}
		if name == "harness.session_id" || name == "harness.turn_id" {
			v.warn("T006", source, name+" changes between events and may defeat rule deduplication")
		}
	default:
		v.add("undeclared-var", source, name+" is not a declared template variable")
	}
}

func isRuntimeTemplateName(name string) bool {
	return styletemplate.IsRuntimeVariable(name)
}

func runtimeReadForTemplate(name string) string {
	if name == "thinking_effort_raw" {
		return "thinking_effort_raw"
	}
	return name
}

func (v *validator) settingDeclared(name string) bool {
	settings, ok := v.pkg.Metadata["settings"].([]any)
	if !ok {
		return false
	}
	for _, entry := range settings {
		if fields := asStringMap(entry); fields != nil && fields["name"] == name {
			return true
		}
	}
	return false
}

func (v *validator) whenGuarantees(rule Rule, name string) bool {
	whenValue := rule.Fields["when"]
	if override, exists := rule.Overrides["when"]; exists {
		whenValue = override
	}
	when := asStringMap(whenValue)
	if when == nil {
		return false
	}
	switch name {
	case "model":
		return when["model"] != nil
	case "thinking_effort", "thinking_effort_raw":
		return when["thinking_effort"] != nil
	default:
		if strings.HasPrefix(name, "harness.") {
			harness := asStringMap(when["harness"])
			return harness[strings.TrimPrefix(name, "harness.")] != nil
		}
	}
	return false
}

func hasKey(values map[string]any, key string) bool {
	_, ok := values[key]
	return ok
}
