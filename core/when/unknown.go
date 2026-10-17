package when

import "strings"

// AllowsUnknown checks the explicit missing-value selector on supported fields.
// It accepts raw YAML alternatives as well as normalized compiled lists.
func AllowsUnknown(field string, value any) bool {
	isUnknown := func(text string) bool {
		switch field {
		case "model":
			return strings.EqualFold(text, "unknown")
		case "thinking_effort":
			return strings.EqualFold(strings.TrimSpace(text), "unknown")
		case "provider", "harness.permission_mode":
			return text == "unknown"
		default:
			return false
		}
	}
	switch values := value.(type) {
	case string:
		return isUnknown(values)
	case []string:
		for _, v := range values {
			if isUnknown(v) {
				return true
			}
		}
	case []any:
		for _, v := range values {
			if text, ok := v.(string); ok && isUnknown(text) {
				return true
			}
		}
	}
	return false
}

// UsesUnknown identifies conditions that need the missing-value index format.
func (rule Rule) UsesUnknown() bool {
	return AllowsUnknown("model", rule.Model) || AllowsUnknown("provider", rule.Provider) || AllowsUnknown("thinking_effort", rule.ThinkingEffort) || AllowsUnknown("harness.permission_mode", rule.Harness["permission_mode"])
}

func matchSelector(field string, alternatives []string, actual string) bool {
	if actual == "" {
		return AllowsUnknown(field, alternatives)
	}
	for _, expected := range alternatives {
		if AllowsUnknown(field, expected) {
			continue
		}
		switch field {
		case "model":
			if matchModel(expected, actual) {
				return true
			}
		case "thinking_effort":
			if strings.EqualFold(expected, actual) {
				return true
			}
		default:
			if expected == actual {
				return true
			}
		}
	}
	return false
}
