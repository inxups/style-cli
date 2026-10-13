package plan

import (
	"github.com/inxups/style-cli/core/spec"
	"sort"
	"strings"
)

func stringValue(value any) string { text, _ := value.(string); return text }

func firstString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func stringMap(value any) map[string]any { result, _ := value.(map[string]any); return result }

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func valuesAsStrings(value any) []string {
	switch values := value.(type) {
	case string:
		return []string{values}
	case []string:
		return append([]string(nil), values...)
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func harnessSelectorMatches(rule spec.Rule, name string) bool {
	harness := stringMap(stringMap(rule.Effective["when"])["harness"])
	value, exists := harness["name"]
	if !exists {
		return true
	}
	for _, candidate := range valuesAsStrings(value) {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}
