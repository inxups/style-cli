package spec

func cloneMap(fields map[string]any) map[string]any {
	cloned := make(map[string]any, len(fields))
	for key, value := range fields {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneMapWithout(fields map[string]any, omitted string) map[string]any {
	cloned := make(map[string]any, len(fields))
	for key, value := range fields {
		if key != omitted {
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
			if stringKey, ok := key.(string); ok {
				converted[stringKey] = item
			}
		}
		return converted
	default:
		return nil
	}
}

func unionReads(values ...any) map[string]any {
	merged := make(map[string]any)
	harness := make([]string, 0)
	seenHarness := make(map[string]bool)
	for _, value := range values {
		fields := asStringMap(value)
		for key, item := range fields {
			switch key {
			case "model", "thinking_effort":
				if enabled, ok := item.(bool); ok && enabled {
					merged[key] = true
				}
			case "harness":
				for _, name := range stringValues(item) {
					if !seenHarness[name] {
						seenHarness[name] = true
						harness = append(harness, name)
					}
				}
			}
		}
	}
	if len(harness) != 0 {
		merged["harness"] = harness
	}
	return merged
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
