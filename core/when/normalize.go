package when

import (
	"fmt"
	"strings"
)

// Normalize converts the YAML-decoded when mapping into its runtime form.
// event is the normalized evaluation point used for the default agent scope.
// Callers validate declarations and event availability separately.
func Normalize(value map[string]any, event string) (Rule, error) {
	rule := Rule{Event: event}
	var err error
	if raw, exists := value["model"]; exists {
		rule.Model, err = stringList(raw)
		if err != nil {
			return Rule{}, fmt.Errorf("when.model: %w", err)
		}
		for i := range rule.Model {
			rule.Model[i] = strings.ToLower(rule.Model[i])
		}
	}
	if raw, exists := value["provider"]; exists {
		rule.Provider, err = stringList(raw)
		if err != nil {
			return Rule{}, fmt.Errorf("when.provider: %w", err)
		}
	}
	if raw, exists := value["thinking_effort"]; exists {
		rule.ThinkingEffort, err = ExpandEffort(raw)
		if err != nil {
			return Rule{}, fmt.Errorf("when.thinking_effort: %w", err)
		}
	}
	if raw, exists := value["thinking_effort_approx"]; exists {
		approx, ok := raw.(bool)
		if !ok {
			return Rule{}, fmt.Errorf("when.thinking_effort_approx must be a boolean")
		}
		rule.ThinkingEffortApprox = &approx
	}
	if raw, exists := value["harness"]; exists {
		harness, ok := stringMap(raw)
		if !ok {
			return Rule{}, fmt.Errorf("when.harness must be a mapping")
		}
		rule.Harness = make(map[string][]string, len(harness))
		for key, item := range harness {
			values, listErr := stringList(item)
			if listErr != nil {
				return Rule{}, fmt.Errorf("when.harness.%s: %w", key, listErr)
			}
			rule.Harness[key] = values
		}
	}
	if raw, exists := value["agent"]; exists {
		rule.Agent, err = stringList(raw)
		if err != nil {
			return Rule{}, fmt.Errorf("when.agent: %w", err)
		}
		for _, agent := range rule.Agent {
			if agent == "any" {
				rule.Agent = []string{"*"}
				break
			}
		}
	} else if event == "subagent.start" {
		rule.Agent = []string{"*"}
	} else {
		rule.Agent = []string{"main"}
	}
	if raw, exists := value["conditions"]; exists {
		items, ok := raw.([]any)
		if !ok {
			return Rule{}, fmt.Errorf("when.conditions must be a list")
		}
		rule.Conditions = make([]Condition, 0, len(items))
		for i, item := range items {
			fields, ok := stringMap(item)
			if !ok {
				return Rule{}, fmt.Errorf("when.conditions[%d] must be a mapping", i)
			}
			field, fieldOK := fields["field"].(string)
			op, opOK := fields["op"].(string)
			pattern, patternOK := fields["pattern"].(string)
			if !fieldOK || !opOK || !patternOK {
				return Rule{}, fmt.Errorf("when.conditions[%d] requires string field, op, and pattern", i)
			}
			rule.Conditions = append(rule.Conditions, Condition{Field: field, Op: op, Pattern: pattern})
		}
	}
	if _, err := Compile(rule); err != nil {
		return Rule{}, err
	}
	return rule, nil
}
