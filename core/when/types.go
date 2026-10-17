package when

// Condition is one hook field predicate. Conditions in a rule are ANDed.
type Condition struct {
	Field   string `json:"field"`
	Op      string `json:"op"`
	Pattern string `json:"pattern"`
}

// Rule is the normalized subset of a STYLE when mapping needed at runtime.
// Lists within one field are alternatives; different fields are conjunctive.
// Nil selector lists are omitted; non-nil empty lists have no alternatives.
// Use Normalize before serialization to retain the event's default agent scope.
type Rule struct {
	Model                []string            `json:"model,omitempty"`
	Provider             []string            `json:"provider,omitempty"`
	ThinkingEffort       []string            `json:"thinking_effort,omitempty"`
	ThinkingEffortApprox *bool               `json:"thinking_effort_approx"`
	Harness              map[string][]string `json:"harness,omitempty"`
	Agent                []string            `json:"agent"`
	Conditions           []Condition         `json:"conditions,omitempty"`
	Event                string              `json:"-"`
}

// Input contains data enriched by the active harness adapter.
type Input struct {
	Provider             string
	ProviderSource       string
	Model                string
	ThinkingEffort       string
	ThinkingEffortApprox *bool
	Harness              map[string]string
	AgentID              string
	AgentType            string
	Fields               FieldReader
}
