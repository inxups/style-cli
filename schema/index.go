// Package schema provides embedded validation for the runtime index format.
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed index.schema.json
var indexSchemaJSON []byte

var (
	resolvedOnce sync.Once
	resolved     *jsonschema.Resolved
	resolveErr   error
)

// ValidateIndex checks raw JSON against the exact schema shipped with STYLE.
func ValidateIndex(data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("index schema: decode JSON: %w", err)
	}
	resolvedOnce.Do(func() {
		var schema jsonschema.Schema
		if err := json.Unmarshal(indexSchemaJSON, &schema); err != nil {
			resolveErr = fmt.Errorf("index schema: decode embedded schema: %w", err)
			return
		}
		resolved, resolveErr = schema.Resolve(nil)
		if resolveErr != nil {
			resolveErr = fmt.Errorf("index schema: resolve embedded schema: %w", resolveErr)
		}
	})
	if resolveErr != nil {
		return resolveErr
	}
	if err := resolved.Validate(document); err != nil {
		return fmt.Errorf("index schema: invalid runtime index: %w", err)
	}
	return nil
}
