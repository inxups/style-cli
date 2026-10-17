package when

import (
	"fmt"
	"strings"
)

var effortTiers = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

// ExpandEffort expands a valid exact/list/comparison effort expression.
// The open value is deliberately excluded from ordered comparisons.
func ExpandEffort(value any) ([]string, error) {
	if text, ok := value.(string); ok {
		text = strings.ToLower(strings.TrimSpace(text))
		if strings.HasPrefix(text, ">=") || strings.HasPrefix(text, "<=") {
			if len(text) <= 2 {
				return nil, fmt.Errorf("empty effort comparison")
			}
			operator, tier := text[:2], strings.TrimSpace(text[2:])
			index := effortIndex(tier)
			if index < 0 {
				return nil, fmt.Errorf("unknown effort tier %q", tier)
			}
			if operator == ">=" {
				return append([]string(nil), effortTiers[index:]...), nil
			}
			return append([]string(nil), effortTiers[:index+1]...), nil
		}
		if text == "open" || text == "unknown" || effortIndex(text) >= 0 {
			return []string{text}, nil
		}
		return nil, fmt.Errorf("unknown effort tier %q", text)
	}
	values, err := stringList(value)
	if err != nil || len(values) == 0 {
		if err == nil {
			err = fmt.Errorf("effort list must not be empty")
		}
		return nil, err
	}
	for i := range values {
		values[i] = strings.ToLower(strings.TrimSpace(values[i]))
		if values[i] != "open" && values[i] != "unknown" && effortIndex(values[i]) < 0 {
			return nil, fmt.Errorf("unknown effort tier %q", values[i])
		}
	}
	return values, nil
}

func effortIndex(value string) int {
	for i, tier := range effortTiers {
		if value == tier {
			return i
		}
	}
	return -1
}
