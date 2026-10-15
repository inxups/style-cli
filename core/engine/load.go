package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/schema"
)

const maxIndexBytes = 16 << 20

// Engine is immutable after construction and safe for concurrent calls.
type Engine struct {
	index   compile.Index
	store   *state.Store
	entries map[string]map[string][]compiledEntry
}

// Load validates an index against the committed JSON Schema before decoding
// and compiling reusable condition matchers.
func Load(reader io.Reader, store *state.Store) (*Engine, error) {
	if reader == nil {
		return nil, fmt.Errorf("engine: index reader is required")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxIndexBytes+1))
	if err != nil {
		return nil, fmt.Errorf("engine: read index: %w", err)
	}
	if len(data) > maxIndexBytes {
		return nil, fmt.Errorf("engine: index exceeds %d bytes", maxIndexBytes)
	}
	if err := schema.ValidateIndex(data); err != nil {
		return nil, err
	}
	var index compile.Index
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("engine: decode index: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("engine: trailing index data")
	}
	return newValidated(index, store)
}

// New validates an in-memory index through its canonical JSON representation
// and compiles conditions once for reuse.
func New(index compile.Index, store *state.Store) (*Engine, error) {
	if store == nil {
		return nil, fmt.Errorf("engine: state store is required")
	}
	data, err := compile.Encode(index)
	if err != nil {
		return nil, fmt.Errorf("engine: encode index: %w", err)
	}
	if err := schema.ValidateIndex(data); err != nil {
		return nil, err
	}
	return newValidated(index, store)
}

// Both public constructors validate their input before building matchers. Load
// already checked the original JSON, so it need not encode and validate it again.
func newValidated(index compile.Index, store *state.Store) (*Engine, error) {
	if store == nil {
		return nil, fmt.Errorf("engine: state store is required")
	}
	engine := &Engine{index: index, store: store, entries: make(map[string]map[string][]compiledEntry)}
	for harness, events := range index.Harnesses {
		engine.entries[harness] = make(map[string][]compiledEntry, len(events))
		for point, entries := range events {
			compiled := make([]compiledEntry, 0, len(entries))
			for _, entry := range entries {
				if entry.Reevaluate && (entry.Target != "system" || point != "prompt.submitted") {
					return nil, fmt.Errorf("engine: reeval is only valid for dynamic system entries at prompt.submitted")
				}
				if entry.Target == "system" && point == "prompt.submitted" && !entry.Reevaluate {
					return nil, fmt.Errorf("engine: dynamic system prompt entry %q must re-evaluate", entry.Entry)
				}
				matcher, err := when.Compile(entry.When)
				if err != nil {
					return nil, fmt.Errorf("engine: compile condition for %q: %w", entry.Entry, err)
				}
				compiled = append(compiled, compiledEntry{entry: entry, matcher: matcher})
			}
			engine.entries[harness][point] = compiled
		}
	}
	return engine, nil
}

// LoadIndexBytes loads a bounded byte slice using the same schema checks.
func LoadIndexBytes(data []byte, store *state.Store) (*Engine, error) {
	return Load(bytes.NewReader(data), store)
}
