package engine

// HasFailClosed deliberately inspects event buckets before state access. Normal
// predicates remain normal predicates; faults cannot reliably evaluate them.
func (e *Engine) HasFailClosed(harness string, points ...string) bool {
	if e == nil {
		return false
	}
	for _, point := range points {
		for _, item := range e.entries[harness][point] {
			if item.entry.Fail == "closed" {
				return true
			}
		}
	}
	return false
}
