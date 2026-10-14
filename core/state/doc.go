// Package state stores STYLE's small per-session runtime state and
// cross-process repeat reservations.
//
// It does not interpret provider payloads or compiled rules. Callers update
// lifecycle counters using normalized event facts and claim deduplication
// keys after rendering model-facing text.
package state
