package spec

import "sort"

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isStringOrStringList(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int32, int64, uint, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func toFloat64(value any) float64 {
	switch number := value.(type) {
	case int:
		return float64(number)
	case int32:
		return float64(number)
	case int64:
		return float64(number)
	case uint:
		return float64(number)
	case uint32:
		return float64(number)
	case uint64:
		return float64(number)
	case float32:
		return float64(number)
	case float64:
		return number
	default:
		return 0
	}
}

func isScalar(value any) bool {
	switch value.(type) {
	case string, bool, int, int32, int64, uint, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}
