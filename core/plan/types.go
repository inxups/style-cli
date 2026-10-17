package plan

import "github.com/inxups/style-cli/core/spec"

// Capability identifies one output behavior an adapter may support at an
// evaluation point.
type Capability string

const (
	AddContext    Capability = "add-context"
	SystemMessage Capability = "system-message"
	Deny          Capability = "deny"
)

// Adapter exposes provider facts to the planner without coupling core policy
// to a harness implementation.
type Adapter interface {
	Name() string
	Supports(event string, capability Capability) bool
	DataSource(event, field string) bool
	// BlindSpot reports whether the entire rule can never match on this adapter.
	// A reason with unobservable=false documents a partial visibility boundary.
	BlindSpot(rule spec.Rule) (reason string, unobservable bool)
	// StaticLocation reports the slot available for a system mode such as append
	// or replace.
	StaticLocation(mode string) (slot string, supported bool)
}

// NativeAdapter is implemented by adapters that can write target: native
// rules (rules §3.10). An error means the mapping cannot be written on this
// harness version or route; the planner then applies the rule's fallback.
type NativeAdapter interface {
	NativeHook(rule spec.NativeRule) (NativeEntry, error)
}

// NativeEntry is the adapter's resolution of one native mapping. It carries
// no command: plan reports never show native commands or bodies.
type NativeEntry struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitempty"`
	Timeout int    `json:"timeout"`
}

// Report contains the deterministic plan for one adapter.
type Report struct {
	Package string     `json:"package"`
	Adapter string     `json:"adapter"`
	Rules   []RulePlan `json:"rules"`
	Blocked bool       `json:"blocked"`
}

// RulePlan is one user rule and its resulting runtime or static placements.
type RulePlan struct {
	Fail             string         `json:"fail,omitempty"`
	Refresh          *RefreshPolicy `json:"refresh,omitempty"`
	DeduplicatedInto string         `json:"deduplicated_into,omitempty"`
	ID               string         `json:"id"`
	Source           string         `json:"source"`
	Target           string         `json:"target"`
	Action           string         `json:"action,omitempty"` // empty for native rules
	Status           Status         `json:"status"`
	Needs            []string       `json:"needs,omitempty"`
	Native           *NativeEntry   `json:"native,omitempty"`
	Placements       []Placement    `json:"placements,omitempty"`
	Reasons          []string       `json:"reasons,omitempty"`
	Fatal            bool           `json:"fatal,omitempty"`
}

// Placement is one event hook or static slot selected by the planner.
type Placement struct {
	Event          string       `json:"event,omitempty"`
	Action         string       `json:"action"`
	Status         Status       `json:"status"`
	Capabilities   []Capability `json:"capabilities,omitempty"`
	Repeat         string       `json:"repeat,omitempty"`
	StaticSlot     string       `json:"static_slot,omitempty"`
	StaticFallback bool         `json:"static_fallback,omitempty"`
	StaticPrefix   string       `json:"-"`
	Reevaluate     bool         `json:"reevaluate,omitempty"`
	DegradedFrom   string       `json:"degraded_from,omitempty"`
	MissingData    []string     `json:"missing_data,omitempty"`
	Warnings       []string     `json:"warnings,omitempty"`
	Reasons        []string     `json:"reasons,omitempty"`
}

// Status describes how a rule or placement will behave.
type Status string

const (
	Native       Status = "native"
	Degraded     Status = "degraded"
	Partial      Status = "partial"
	Skipped      Status = "skipped"
	Never        Status = "never-trigger"
	NativeHook   Status = "native-hook"
	InstallError Status = "install-error"
)

// Requirement is the adapter-independent demand derived from a merged rule.
// AnyCapabilities is an OR set; AllCapabilities must all be present.
type Requirement struct {
	Fail              string
	Refresh           int
	RuleID            string
	Target            string
	Action            string
	Events            []string
	AllCapabilities   []Capability
	AnyCapabilities   []Capability
	Needs             []string
	ConditionNeeds    []string
	UnknownNeeds      []string
	Fallback          string
	Mode              string
	Repeat            string
	StaticPrefix      string
	StaticBody        bool
	HasWhen           bool
	ProviderCondition bool
}

// RefreshPolicy describes submitted-prompt cadence without revealing content.
type RefreshPolicy struct {
	Interval    int      `json:"interval"`
	Clock       string   `json:"clock"`
	Cadence     string   `json:"cadence"`
	MatchedOnly bool     `json:"matched_only"`
	Limits      []string `json:"limits"`
}

func DescribeRefresh(interval int) *RefreshPolicy {
	return &RefreshPolicy{Interval: interval, Clock: "prompt.submitted", Cadence: "first match, then N submitted turns since last emission", MatchedOnly: true,
		Limits: []string{"prompt-count-fallback without native turn IDs", "no child cadence without child prompt callbacks", "due output waits for its original matching point", "full per-emission budget"}}
}
