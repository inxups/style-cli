package when

import "fmt"

// Matcher holds a normalized rule and precompiled condition expressions.
// Compile once after loading a rule, then reuse Match for each evaluation.
type Matcher struct {
	rule       Rule
	conditions []predicate
}

// Compile prepares a normalized rule for evaluation. It rejects invalid
// regular expressions, operators, effort tiers, and model glob syntax.
func Compile(rule Rule) (*Matcher, error) {
	matcher := &Matcher{rule: cloneRule(rule)}
	if matcher.rule.Agent == nil {
		if matcher.rule.Event == "subagent.start" {
			matcher.rule.Agent = []string{"*"}
		} else {
			matcher.rule.Agent = []string{"main"}
		}
	}
	for _, pattern := range rule.Model {
		if err := validateModelGlob(pattern); err != nil {
			return nil, fmt.Errorf("when.model: %w", err)
		}
	}
	if rule.ThinkingEffort != nil {
		if _, err := ExpandEffort(rule.ThinkingEffort); err != nil {
			return nil, fmt.Errorf("when.thinking_effort: %w", err)
		}
	}
	for i, condition := range rule.Conditions {
		compiled, err := compilePredicate(condition)
		if err != nil {
			return nil, fmt.Errorf("when.conditions[%d]: %w", i, err)
		}
		matcher.conditions = append(matcher.conditions, compiled)
	}
	return matcher, nil
}

// Match is a convenience for single evaluations. Repeated evaluations should
// use Compile and Matcher.Match to reuse compiled expressions.
func Match(rule Rule, input Input) bool {
	matcher, err := Compile(rule)
	return err == nil && matcher.Match(input)
}

// Match requires every configured field and every condition to match.
// Empty selector lists have no alternatives, so cannot match.
func (m *Matcher) Match(input Input) bool {
	if m == nil {
		return false
	}
	rule := m.rule
	if rule.Provider != nil {
		// M1 has no provider data source; a declared provider never matches.
		return false
	}
	if rule.Model != nil && (input.Model == "" || !anyModelMatches(rule.Model, input.Model)) {
		return false
	}
	if rule.ThinkingEffort != nil && (input.ThinkingEffort == "" || !containsFold(rule.ThinkingEffort, input.ThinkingEffort)) {
		return false
	}
	if rule.ThinkingEffortApprox != nil && (input.ThinkingEffortApprox == nil || *rule.ThinkingEffortApprox != *input.ThinkingEffortApprox) {
		return false
	}
	for key, values := range rule.Harness {
		actual, ok := input.Harness[key]
		if !ok || actual == "" || !contains(values, actual) {
			return false
		}
	}
	if !contains(rule.Agent, "*") {
		if input.AgentID == "" {
			if !contains(rule.Agent, "main") {
				return false
			}
		} else if input.AgentType == "" || input.AgentType == "main" || !contains(rule.Agent, input.AgentType) {
			return false
		}
	}
	for _, condition := range m.conditions {
		if input.Fields == nil {
			return false
		}
		actual, available := input.Fields.ReadConditionField(condition.Field)
		if !available || !condition.match(actual) {
			return false
		}
	}
	return true
}

func cloneRule(rule Rule) Rule {
	clone := rule
	if rule.Model != nil {
		clone.Model = append([]string{}, rule.Model...)
	}
	if rule.Provider != nil {
		clone.Provider = append([]string{}, rule.Provider...)
	}
	if rule.ThinkingEffort != nil {
		clone.ThinkingEffort = append([]string{}, rule.ThinkingEffort...)
	}
	if rule.ThinkingEffortApprox != nil {
		value := *rule.ThinkingEffortApprox
		clone.ThinkingEffortApprox = &value
	}
	if rule.Agent != nil {
		clone.Agent = append([]string{}, rule.Agent...)
	}
	clone.Harness = make(map[string][]string, len(rule.Harness))
	for key, values := range rule.Harness {
		clone.Harness[key] = append([]string{}, values...)
	}
	clone.Conditions = append([]Condition(nil), rule.Conditions...)
	return clone
}
