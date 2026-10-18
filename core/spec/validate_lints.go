package spec

import (
	"strconv"

	stylewarn "github.com/inxups/style-cli/core/warn"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

func (v *validator) validateLint(rule Rule, source string) {
	if estimate, ok := modelEstimate(rule, v.pkg); ok {
		limit := v.effectiveRuleMaxTokens(rule)
		if float64(estimate.Tokens) > limit {
			message := "estimated model input exceeds max_tokens (" + strconv.Itoa(estimate.Tokens) + " > " + strconv.FormatFloat(limit, 'f', -1, 64) + ")"
			if v.budgetSeverity() == SeverityError {
				v.add("T001", source, message)
			} else {
				v.warn("T001", source, message)
			}
		}
	}
	if v.effectiveRuleString(rule, "action", "inject") == "block" && estimateTemplateTokens(rule.Body, v.pkg, rule) > 40 {
		v.warn("T005", source, "block reason exceeds the 40-token recommendation")
	}
}

func modelEstimate(rule Rule, pkg *Package) (tokenestimate.Count, bool) {
	action := effectiveRuleText(rule, pkg, "action", "inject")
	target := effectiveRuleText(rule, pkg, "target", "")
	if action == "block" {
		event := effectiveRuleText(rule, pkg, "event", "")
		if target == "event" && event != "tool.pre" {
			return tokenestimate.Count{}, true
		}
	}
	source := rule.Body
	if action == "warn" {
		if explicit, exists := effectiveRuleValue(rule, pkg, "model_text"); exists {
			text, ok := explicit.(string)
			if !ok {
				return tokenestimate.Count{}, false
			}
			source = text
		} else {
			rendered, err := tokenestimate.RenderForEstimate(source, estimateContext(pkg))
			if err != nil {
				return tokenestimate.Count{}, false
			}
			return tokenestimate.CountText(stylewarn.FirstSentence(rendered)), true
		}
	}
	count, err := tokenestimate.EstimateTemplate(source, estimateContext(pkg))
	if err != nil {
		return tokenestimate.Count{}, false
	}
	return count, true
}

func effectiveRuleText(rule Rule, pkg *Package, key, fallback string) string {
	value, ok := effectiveRuleValue(rule, pkg, key)
	if !ok {
		return fallback
	}
	text, _ := value.(string)
	if text == "" {
		return fallback
	}
	return text
}

func effectiveRuleValue(rule Rule, pkg *Package, key string) (any, bool) {
	if value, ok := rule.Overrides[key]; ok {
		return value, true
	}
	if value, ok := rule.Fields[key]; ok {
		return value, true
	}
	if pkg != nil {
		if defaults := asStringMap(pkg.Metadata["defaults"]); defaults != nil {
			value, ok := defaults[key]
			return value, ok
		}
	}
	return nil, false
}

func estimateContext(pkg *Package) tokenestimate.Context {
	var context tokenestimate.Context
	if pkg != nil {
		context.Package = pkg.Metadata
	}
	return context
}

func (v *validator) effectiveRuleMaxTokens(rule Rule) float64 {
	limit := float64(400)
	if budget := asStringMap(v.pkg.Metadata["budget"]); budget != nil {
		if value, ok := budget["per_rule"]; ok {
			limit = toFloat64(value)
		}
	}
	if defaults := asStringMap(v.pkg.Metadata["defaults"]); defaults != nil {
		if value, ok := defaults["max_tokens"]; ok {
			limit = toFloat64(value)
		}
	}
	if value, ok := rule.Fields["max_tokens"]; ok {
		limit = toFloat64(value)
	}
	if value, ok := rule.Overrides["max_tokens"]; ok {
		limit = toFloat64(value)
	}
	return limit
}

func (v *validator) budgetSeverity() string {
	if budget := asStringMap(v.pkg.Metadata["budget"]); budget != nil {
		if value, ok := budget["on"].(string); ok && value == SeverityError {
			return SeverityError
		}
	}
	return SeverityWarning
}

// estimateTokens is the offline estimate described by STYLE-spec §7.4. It is
// kept as a local alias so spec tests can verify the lint threshold directly.
func estimateTokens(text string) int { return tokenestimate.Estimate(text) }

func estimateTemplateTokens(text string, pkg *Package, rule Rule) int {
	count, err := tokenestimate.EstimateTemplate(text, estimateContext(pkg))
	if err != nil {
		return tokenestimate.Estimate(text)
	}
	return count.Tokens
}
