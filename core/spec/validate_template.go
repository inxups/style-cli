package spec

import (
	"fmt"
	"regexp"
	"strings"
)

var templateNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
var eventTemplateNames = []string{"prompt", "tool.name", "tool.input", "tool.added", "tool.output", "mcp.server", "mcp.tool", "skill.name"}

func (v *validator) validateTemplate(text, source string, rule Rule) {
	for offset := 0; offset < len(text); {
		start := strings.Index(text[offset:], "{{")
		if start < 0 {
			return
		}
		start += offset
		if start > 0 && text[start-1] == '\\' {
			offset = start + 2
			continue
		}
		end := strings.Index(text[start+2:], "}}")
		if end < 0 {
			v.add("invalid-template", source, "template expression has no closing }}")
			return
		}
		end += start + 2
		expression := strings.TrimSpace(text[start+2 : end])
		name, hasDefault, valid := parseTemplateExpression(expression)
		if !valid {
			v.add("invalid-template", source, fmt.Sprintf("invalid template expression %q", expression))
		} else {
			v.validateTemplateName(name, hasDefault, source, rule)
		}
		offset = end + 2
	}
}

func parseTemplateExpression(expression string) (name string, hasDefault, valid bool) {
	parts := strings.Split(expression, "|")
	name = strings.TrimSpace(parts[0])
	if !templateNamePattern.MatchString(name) {
		return "", false, false
	}
	if len(parts) == 1 {
		return name, false, true
	}
	if len(parts) != 2 {
		return "", false, false
	}
	defaultExpr := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(defaultExpr, "default:") {
		return "", false, false
	}
	value := strings.TrimSpace(strings.TrimPrefix(defaultExpr, "default:"))
	if len(value) < 2 || !((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return "", false, false
	}
	return name, true, true
}

func (v *validator) validateTemplateName(name string, hasDefault bool, source string, rule Rule) {
	for _, eventName := range eventTemplateNames {
		if name == eventName || eventName == "tool.input" && strings.HasPrefix(name, "tool.input.") {
			v.add("event-var-in-template", source, name+" cannot be used in a template")
			return
		}
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
	return name == "model" || name == "thinking_effort" || name == "thinking_effort_raw" || strings.HasPrefix(name, "harness.")
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
