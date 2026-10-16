package when

import (
	"fmt"
	"regexp"
	"strings"
)

type predicate struct {
	Condition
	regex *regexp.Regexp
}

func compilePredicate(condition Condition) (predicate, error) {
	compiled := predicate{Condition: condition}
	switch condition.Op {
	case "regex", "shell_regex":
		regex, err := regexp.Compile(condition.Pattern)
		if err != nil {
			return predicate{}, err
		}
		compiled.regex = regex
	case "contains", "equals", "not_contains", "starts_with", "ends_with":
	default:
		return predicate{}, fmt.Errorf("unknown condition operator %q", condition.Op)
	}
	return compiled, nil
}

func (p predicate) match(actual string) bool {
	switch p.Op {
	case "regex":
		return p.regex.MatchString(actual)
	case "shell_regex":
		for _, command := range shellCommands(actual) {
			if p.regex.MatchString(command) {
				return true
			}
		}
		return false
	case "contains":
		return strings.Contains(actual, p.Pattern)
	case "equals":
		return actual == p.Pattern
	case "not_contains":
		return !strings.Contains(actual, p.Pattern)
	case "starts_with":
		return strings.HasPrefix(actual, p.Pattern)
	case "ends_with":
		return strings.HasSuffix(actual, p.Pattern)
	default:
		return false
	}
}
