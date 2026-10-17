package engine

// NeedsProvider enables provider resolution only for the active harness.
func (e *Engine) NeedsProvider(harness string) bool {
	if e == nil {
		return false
	}
	for _, entries := range e.entries[harness] {
		for _, entry := range entries {
			if entry.entry.HasProvider() {
				return true
			}
		}
	}
	return false
}
