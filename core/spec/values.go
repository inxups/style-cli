package spec

func cloneMap(fields map[string]any) map[string]any {
	cloned := make(map[string]any, len(fields))
	for key, value := range fields {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneMapWithout(fields map[string]any, omitted ...string) map[string]any {
	cloned := make(map[string]any, len(fields))
	excluded := stringSet(omitted...)
	for key, value := range fields {
		if !excluded[key] {
			cloned[key] = cloneValue(value)
		}
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case map[any]any:
		cloned := make(map[any]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func asStringMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			stringKey, ok := key.(string)
			if !ok {
				return nil
			}
			converted[stringKey] = item
		}
		return converted
	default:
		return nil
	}
}

// hasNonStringMapKey detects YAML mappings that would otherwise lose entries
// when converted from map[any]any to map[string]any.
func hasNonStringMapKey(value any) bool {
	switch typed := value.(type) {
	case map[any]any:
		for key, item := range typed {
			if _, ok := key.(string); !ok || hasNonStringMapKey(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if hasNonStringMapKey(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if hasNonStringMapKey(item) {
				return true
			}
		}
	}
	return false
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if stringValue, ok := item.(string); ok {
				values = append(values, stringValue)
			}
		}
		return values
	default:
		return nil
	}
}
