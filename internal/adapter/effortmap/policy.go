package effortmap

import (
	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
)

// Policy is an immutable compiled user snapshot, never an ambient file reader.
type Policy struct {
	rows   map[string]string
	digest string
}

func NewPolicy(harness string, mapping compile.EffortMapping) Policy {
	detached := mapping.Clone()
	return Policy{rows: detached[harness], digest: detached.SHA256(harness)}
}
func (p Policy) Active(previous state.Session) bool {
	return p.digest != "" || previous.EffortMappingSHA256 != ""
}

// Apply remaps the latest native raw, including cached raw when only the mapping
// changes. A missing raw never becomes a canonical value through user policy.
func (p Policy) Apply(update state.EventUpdate, previous state.Session, builtin Table) state.EventUpdate {
	if !p.Active(previous) {
		return update
	}
	raw := update.EffortRaw
	if raw == "" {
		raw = previous.EffortRaw
	}
	value := builtin.Resolve(raw)
	source := "builtin"
	if value.Effort == "" {
		source = "unmapped"
	}
	if tier, ok := p.rows[normalize(raw)]; ok {
		approx := true
		value = Value{Effort: tier, EffortRaw: raw, EffortApprox: &approx}
		source = "user"
	}
	update.ThinkingEffort = value.Effort
	update.EffortRaw = value.EffortRaw
	update.EffortApprox = value.EffortApprox
	update.EffortMappingResolved = true
	update.EffortMappingSHA256 = p.digest
	if p.digest != "" {
		update.EffortMappingSource = source
	}
	return update
}
