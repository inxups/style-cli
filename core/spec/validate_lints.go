package spec

import (
	"fmt"
	"strings"

	styletemplate "github.com/inxups/style-cli/core/template"
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
	parsed, err := styletemplate.Parse(text)
	if err != nil {
		return tokenestimate.Estimate(text)
	}
	for _, part := range parsed.Parts() {
		if part.Kind == styletemplate.TextPart {
			rendered.WriteString(part.Text)
			continue
		}
		if isRuntimeTemplateName(part.Name) {
			length := 64
			if part.Name == "harness.cwd" {
				length = 128
			}
			rendered.WriteString(strings.Repeat("x", length))
		} else if value, ok := staticTemplateValue(part.Name, pkg, rule); ok {
			rendered.WriteString(value)
		} else if part.HasDefault {
			rendered.WriteString(part.Default)
		} else {
			rendered.WriteString("")
		}
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
