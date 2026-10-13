package when

import "encoding/json"

type wireRule struct {
	Model                *[]string           `json:"model,omitempty"`
	Provider             *[]string           `json:"provider,omitempty"`
	ThinkingEffort       *[]string           `json:"thinking_effort,omitempty"`
	ThinkingEffortApprox *bool               `json:"thinking_effort_approx"`
	Harness              map[string][]string `json:"harness,omitempty"`
	Agent                []string            `json:"agent"`
	Conditions           []Condition         `json:"conditions,omitempty"`
}

// MarshalJSON preserves the distinction between omitted and empty selectors.
func (rule Rule) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireRule{
		Model:                optionalList(rule.Model),
		Provider:             optionalList(rule.Provider),
		ThinkingEffort:       optionalList(rule.ThinkingEffort),
		ThinkingEffortApprox: rule.ThinkingEffortApprox,
		Harness:              rule.Harness,
		Agent:                rule.Agent,
		Conditions:           rule.Conditions,
	})
}

func (rule *Rule) UnmarshalJSON(data []byte) error {
	var decoded wireRule
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	rule.Model = listValue(decoded.Model)
	rule.Provider = listValue(decoded.Provider)
	rule.ThinkingEffort = listValue(decoded.ThinkingEffort)
	rule.ThinkingEffortApprox = decoded.ThinkingEffortApprox
	rule.Harness = decoded.Harness
	rule.Agent = decoded.Agent
	rule.Conditions = decoded.Conditions
	rule.Event = ""
	return nil
}

func optionalList(values []string) *[]string {
	if values == nil {
		return nil
	}
	return &values
}

func listValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
