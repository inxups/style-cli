package tokens

import (
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

// Unit identifies the budget unit used by a harness limit.
type Unit string

const (
	TokensUnit Unit = "tokens"
	CharsUnit  Unit = "chars"
)

// Estimate is an offline model-input estimate. Chars counts Unicode code
// points, while Tokens uses STYLE's conservative formula.
type Estimate struct {
	Tokens int `json:"tokens"`
	Chars  int `json:"chars"`
}

// Limits are package budgets. A zero package limit selects the STYLE default.
type Limits struct {
	PerRule  float64
	PerEvent float64
	AlwaysOn float64
}

// InputLimit is an adapter's per-bucket hard input limit for T003.
type InputLimit struct {
	Value int
	Unit  Unit
}

// Bucket is one native hook event/tool combination for budget accounting.
type Bucket struct {
	Key   string
	Label string
}

// Layout maps one planned placement to the native output buckets it can reach.
// Adapters own this mapping because one hook may combine several STYLE events.
type Layout interface {
	Buckets(rule spec.Rule, placement plan.Placement) []Bucket
}

// UnifiedLayout groups by STYLE evaluation point. It is useful for core-only
// analysis before an adapter supplies native event/tool grouping.
type UnifiedLayout struct{}

func (UnifiedLayout) Buckets(_ spec.Rule, placement plan.Placement) []Bucket {
	if placement.Event != "" {
		return []Bucket{{Key: placement.Event, Label: placement.Event}}
	}
	if placement.StaticSlot != "" {
		key := "static:" + placement.StaticSlot
		return []Bucket{{Key: key, Label: key}}
	}
	return nil
}

// Diagnostic is a stable token or budget finding.
type Diagnostic struct {
	Code     string `json:"code"`
	RuleID   string `json:"rule,omitempty"`
	Event    string `json:"event,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// RuleEstimate records the largest model-facing placement for one rule.
type RuleEstimate struct {
	RuleID   string      `json:"rule"`
	Action   string      `json:"action"`
	Status   plan.Status `json:"status"`
	Estimate Estimate    `json:"estimate"`
	Events   []string    `json:"events,omitempty"`
}

// EventEstimate is the worst-case sum for one unified evaluation point.
type EventEstimate struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Estimate   Estimate `json:"estimate"`
	Block      Estimate `json:"block,omitempty"`
	Other      Estimate `json:"other,omitempty"`
	HasBlock   bool     `json:"has_block,omitempty"`
	BlockCount int      `json:"block_count,omitempty"`
	OtherCount int      `json:"other_count,omitempty"`
}

// Report contains deterministic per-rule, per-event, always-on, and lint
// results. Text itself is deliberately absent.
type Report struct {
	Rules       []RuleEstimate  `json:"rules"`
	Events      []EventEstimate `json:"events"`
	AlwaysOn    Estimate        `json:"always_on"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}
