package spec

import (
	"fmt"
	"strings"

	"github.com/inxups/style-cli/internal/tokenestimate"
)

func (v *validator) validateLint(rule Rule, source string) {
	if v.effectiveRuleString(rule, "action", "inject") == "block" && estimateTemplateTokens(rule.Body, v.pkg, rule) > 40 {
		v.warn("T005", source, "block reason exceeds the 40-token recommendation")
	}
}

// estimateTokens is the offline estimate described by STYLE-spec §7.4. It is
// kept as a local alias so spec tests can verify the lint threshold directly.
func estimateTokens(text string) int { return tokenestimate.Estimate(text) }

func estimateTemplateTokens(text string, pkg *Package, rule Rule) int {
	var rendered strings.Builder
	for offset := 0; offset < len(text); {
		start := strings.Index(text[offset:], "{{")
		if start < 0 {
			rendered.WriteString(text[offset:])
			break
		}
		start += offset
		if start > 0 && text[start-1] == '\\' {
			rendered.WriteString(text[offset : start+2])
			offset = start + 2
			continue
		}
		end := strings.Index(text[start+2:], "}}")
		if end < 0 {
			rendered.WriteString(text[offset:])
			break
		}
		end += start + 2
		rendered.WriteString(text[offset:start])
		expression := strings.TrimSpace(text[start+2 : end])
		name, hasDefault, valid := parseTemplateExpression(expression)
		if valid && isRuntimeTemplateName(name) {
			length := 64
			if name == "harness.cwd" {
				length = 128
			}
			rendered.WriteString(strings.Repeat("x", length))
		} else if value, ok := staticTemplateValue(name, pkg, rule); ok {
			rendered.WriteString(value)
		} else if hasDefault {
			rendered.WriteString(templateDefaultValue(expression))
		} else {
			rendered.WriteString("")
		}
		offset = end + 2
	}
	return tokenestimate.Estimate(rendered.String())
}

func staticTemplateValue(name string, pkg *Package, rule Rule) (string, bool) {
	switch {
	case strings.HasPrefix(name, "vars."):
		vars := asStringMap(pkg.Metadata["vars"])
		value, ok := vars[strings.TrimPrefix(name, "vars.")]
		if ok {
			return fmt.Sprint(value), true
		}
	case strings.HasPrefix(name, "settings."):
		settings, _ := pkg.Metadata["settings"].([]any)
		settingName := strings.TrimPrefix(name, "settings.")
		for _, setting := range settings {
			fields := asStringMap(setting)
			if fields != nil && fields["name"] == settingName {
				if value, exists := fields["default"]; exists {
					return fmt.Sprint(value), true
				}
				return "", true
			}
		}
	case name == "package.name" || name == "package.version":
		value, ok := pkg.Metadata[strings.TrimPrefix(name, "package.")]
		if ok {
			return fmt.Sprint(value), true
		}
	case name == "rule.id":
		value, ok := rule.Fields["id"]
		if !ok {
			value, ok = rule.Overrides["id"]
		}
		if ok {
			return fmt.Sprint(value), true
		}
	}
	return "", false
}

func templateDefaultValue(expression string) string {
	parts := strings.SplitN(expression, "|", 2)
	if len(parts) != 2 {
		return ""
	}
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[1]), "default:"))
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	return ""
}
