package when

import (
	"fmt"
	"sort"
)

// Matcher holds a normalized rule and precompiled condition expressions.
// Compile once after loading a rule, then reuse Match for each evaluation.
type Matcher struct {
	rule        Rule
	conditions  []predicate
	harnessKeys []string
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
		if err := ValidateModelGlob(pattern); err != nil {
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
	for key := range matcher.rule.Harness {
		matcher.harnessKeys = append(matcher.harnessKeys, key)
	}
	sort.Strings(matcher.harnessKeys)
	return matcher, nil
}

// Match is a convenience for single evaluations. Repeated evaluations should
// use Compile and Matcher.Match to reuse compiled expressions.
func Match(rule Rule, input Input) bool {
	matcher, err := Compile(rule)
	return err == nil && matcher.Match(input)
}

// Check contains only predicate availability and outcome, never input text.
type Check struct {
	Field     string `json:"field"`
	Available bool   `json:"available"`
	Result    string `json:"result"`
	Source    string `json:"source"`
}

// Match requires every configured field and condition to match.
// Empty selector lists have no alternatives.
func (m *Matcher) Match(input Input) bool { return m.match(input, nil) }

// MatchWithTrace follows the same short circuit evaluation as Match.
func (m *Matcher) MatchWithTrace(input Input) (bool, []Check) {
	checks := make([]Check, 0)
	matched := m.match(input, &checks)
	return matched, checks
}

func (m *Matcher) match(input Input, checks *[]Check) bool {
	if m == nil {
		return false
	}
	check := func(field string, available, matched bool) bool {
		if checks != nil {
			source := "unavailable"
			if available {
				source = "input"
			}
			result := "pass"
			if !available {
				result = "missing-data"
			} else if !matched {
				result = "predicate-false"
			}
			*checks = append(*checks, Check{Field: field, Available: available, Result: result, Source: source})
		}
		return available && matched
	}
	rule := m.rule
	if rule.Provider != nil && !check("provider", false, false) {
		return false
	}
	if rule.Model != nil && !check("model", input.Model != "", input.Model != "" && anyModelMatches(rule.Model, input.Model)) {
		return false
	}
	if rule.ThinkingEffort != nil && !check("thinking_effort", input.ThinkingEffort != "", input.ThinkingEffort != "" && containsFold(rule.ThinkingEffort, input.ThinkingEffort)) {
		return false
	}
	if rule.ThinkingEffortApprox != nil && !check("thinking_effort_approx", input.ThinkingEffortApprox != nil, input.ThinkingEffortApprox != nil && *rule.ThinkingEffortApprox == *input.ThinkingEffortApprox) {
		return false
	}
	for _, key := range m.harnessKeys {
		actual, ok := input.Harness[key]
		if !check("harness."+key, ok && actual != "", ok && actual != "" && contains(rule.Harness[key], actual)) {
			return false
		}
	}
	if !contains(rule.Agent, "*") {
		matched := contains(rule.Agent, "main")
		if input.AgentID != "" {
			matched = input.AgentType != "" && input.AgentType != "main" && contains(rule.Agent, input.AgentType)
		}
		if !check("agent", input.AgentID == "" || input.AgentType != "", matched) {
			return false
		}
	}
	for index, condition := range m.conditions {
		actual, available := "", false
		if input.Fields != nil {
			actual, available = input.Fields.ReadConditionField(condition.Field)
		}
		if !check(fmt.Sprintf("conditions[%d].%s", index, condition.Field), available, available && condition.match(actual)) {
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
