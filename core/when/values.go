package when

import (
	"fmt"
	"strings"
)

func stringList(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []string:
		return append([]string{}, typed...), nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected strings")
			}
			values = append(values, text)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("expected a string or list of strings")
	}
}

func stringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			text, ok := key.(string)
			if !ok {
				return nil, false
			}
			result[text] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsFold(values []string, candidate string) bool {
	candidate = strings.ToLower(candidate)
	for _, value := range values {
		if strings.ToLower(value) == candidate {
			return true
		}
	}
	return false
}
