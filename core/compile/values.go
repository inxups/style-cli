package compile

import (
	"fmt"
	"math"
	"strconv"

	"github.com/inxups/style-cli/core/spec"
)

func text(value any) string { result, _ := value.(string); return result }

func stringMap(value any) map[string]any { result, _ := value.(map[string]any); return result }

func stringsValue(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{typed}, nil
	case []string:
		return append([]string{}, typed...), nil
	case []any:
		result := make([]string, 0, len(typed))
		for _, value := range typed {
			item, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("expected a string selector")
			}
			result = append(result, item)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected a string or selector list")
	}
}

func ruleOrder(rule spec.Rule, sequence int) (Order, error) {
	value := rule.Effective["priority"]
	if value == nil {
		value = 50
	}
	priority, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil || math.IsNaN(priority) || math.IsInf(priority, 0) {
		return Order{}, fmt.Errorf("priority must be a finite number")
	}
	return Order{Priority: priority, PackageSeq: sequence, RuleIndex: rule.Index}, nil
}

// withSettings overlays collected values into copied setting declarations.
// This also supplies core/tokens with exactly the compile-time rendering values.
func withSettings(pkg *spec.ResolvedPackage, supplied map[string]string) (*spec.ResolvedPackage, error) {
	result, source := *pkg, *pkg.Package
	metadata := make(map[string]any, len(source.Metadata))
	for key, value := range source.Metadata {
		metadata[key] = value
	}
	var settings []any
	seen := make(map[string]bool)
	declared, _ := metadata["settings"].([]any)
	for _, raw := range declared {
		fields := stringMap(raw)
		name := text(fields["name"])
		if name == "" {
			return nil, fmt.Errorf("setting name is required")
		}
		seen[name] = true
		copy := make(map[string]any, len(fields))
		for key, value := range fields {
			copy[key] = value
		}
		if value, ok := supplied[name]; ok {
			copy["default"] = value
		} else if _, ok := copy["default"]; !ok {
			return nil, fmt.Errorf("setting %q requires an installation value", name)
		}
		settings = append(settings, copy)
	}
	for name := range supplied {
		if !seen[name] {
			return nil, fmt.Errorf("unknown setting %q", name)
		}
	}
	metadata["settings"] = settings
	source.Metadata, result.Package = metadata, &source
	return &result, nil
}
