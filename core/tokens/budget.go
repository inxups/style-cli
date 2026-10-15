package tokens

import (
	"math"
	"strconv"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

func limitsFromPackage(pkg *spec.ResolvedPackage) Limits {
	limits := Limits{PerRule: DefaultPerRule, PerEvent: DefaultPerEvent, AlwaysOn: DefaultAlwaysOn}
	if pkg == nil || pkg.Package == nil {
		return limits
	}
	if budget := stringMap(pkg.Package.Metadata["budget"]); budget != nil {
		limits.PerRule = positiveLimit(budget["per_rule"], limits.PerRule)
		limits.PerEvent = positiveLimit(budget["per_event"], limits.PerEvent)
		limits.AlwaysOn = positiveLimit(budget["always_on"], limits.AlwaysOn)
	}
	return limits
}

// PackageLimits returns the effective per-rule, per-event, and always-on
// limits for a resolved package, including STYLE defaults.
func PackageLimits(pkg *spec.ResolvedPackage) Limits { return limitsFromPackage(pkg) }

func effectiveAction(rule spec.Rule, pkg *spec.ResolvedPackage) string {
	if value, ok := effectiveValue(rule, pkg, "action"); ok {
		if action, ok := value.(string); ok && action != "" {
			return action
		}
	}
	return "inject"
}

func effectiveValue(rule spec.Rule, pkg *spec.ResolvedPackage, key string) (any, bool) {
	if value, ok := rule.Effective[key]; ok {
		return value, true
	}
	if value, ok := rule.Overrides[key]; ok {
		return value, true
	}
	if value, ok := rule.Fields[key]; ok {
		return value, true
	}
	if pkg != nil && pkg.Package != nil {
		if defaults := stringMap(pkg.Package.Metadata["defaults"]); defaults != nil {
			value, ok := defaults[key]
			return value, ok
		}
	}
	return nil, false
}

func contextFor(rule spec.Rule, pkg *spec.ResolvedPackage) tokenestimate.Context {
	context := tokenestimate.Context{Rule: rule.Fields, Overrides: rule.Overrides}
	if pkg != nil && pkg.Package != nil {
		context.Package = pkg.Package.Metadata
	}
	return context
}

func ruleID(rule spec.Rule) string {
	if value, ok := rule.Effective["id"].(string); ok && value != "" {
		return value
	}
	if value, ok := rule.Fields["id"].(string); ok {
		return value
	}
	return rule.Source
}

func stringMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func nonnegativeLimit(value any, fallback float64) float64 {
	switch number := value.(type) {
	case int:
		if number >= 0 {
			return float64(number)
		}
	case int32:
		if number >= 0 {
			return float64(number)
		}
	case int64:
		if number >= 0 {
			return float64(number)
		}
	case uint:
		return float64(number)
	case uint32:
		return float64(number)
	case uint64:
		return float64(number)
	case float32:
		if number >= 0 && !math.IsInf(float64(number), 0) && !math.IsNaN(float64(number)) {
			return float64(number)
		}
	case float64:
		if number >= 0 && !math.IsInf(number, 0) && !math.IsNaN(number) {
			return number
		}
	}
	return fallback
}

func positiveLimit(value any, fallback float64) float64 {
	result := nonnegativeLimit(value, fallback)
	if result == 0 {
		return fallback
	}
	return result
}

func budgetSeverity(pkg *spec.ResolvedPackage) string {
	if budget := stringMap(pkg.Package.Metadata["budget"]); budget != nil {
		if value, ok := budget["on"].(string); ok && value == "error" {
			return "error"
		}
	}
	return "warning"
}

func formatLimit(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
