// Package effortmap maps provider-native effort values into STYLE's common
// tier vocabulary without making provider-specific decisions in core.
package effortmap

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

var tiers = map[string]bool{
	"none": true, "minimal": true, "low": true, "medium": true,
	"high": true, "xhigh": true, "max": true, "open": true,
}

// Table is the harness-owned mapping data consumed by all adapters.
type Table struct {
	Harness string            `yaml:"harness"`
	Exact   []string          `yaml:"exact"`
	Aliases map[string]string `yaml:"aliases"`
}

// Value contains the canonical tier and the original native value. A nil
// Approx means the source did not map to a known tier.
type Value struct {
	Effort       string
	EffortRaw    string
	EffortApprox *bool
}

// Parse reads and validates one effort mapping table.
func Parse(reader io.Reader) (Table, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	var table Table
	if err := decoder.Decode(&table); err != nil {
		return Table{}, fmt.Errorf("effortmap: decode table: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Table{}, fmt.Errorf("effortmap: multiple YAML documents are not allowed")
		}
		return Table{}, fmt.Errorf("effortmap: invalid trailing YAML: %w", err)
	}
	if err := table.Validate(); err != nil {
		return Table{}, err
	}
	return table, nil
}

// Validate rejects ambiguous or invalid map definitions.
func (table Table) Validate() error {
	if strings.TrimSpace(table.Harness) == "" {
		return fmt.Errorf("effortmap: harness is required")
	}
	seen := make(map[string]bool, len(table.Exact)+len(table.Aliases))
	for _, raw := range table.Exact {
		key := normalize(raw)
		if key == "" {
			return fmt.Errorf("effortmap: exact values must not be empty")
		}
		if seen[key] {
			return fmt.Errorf("effortmap: duplicate value %q", raw)
		}
		seen[key] = true
		if !tiers[key] {
			return fmt.Errorf("effortmap: exact value %q is not a STYLE tier", raw)
		}
	}
	for raw, tier := range table.Aliases {
		key := normalize(raw)
		if key == "" || seen[key] {
			return fmt.Errorf("effortmap: empty or duplicate alias %q", raw)
		}
		if !tiers[normalize(tier)] {
			return fmt.Errorf("effortmap: alias %q targets unknown tier %q", raw, tier)
		}
		seen[key] = true
	}
	return nil
}

// Resolve maps known native values case-insensitively. Unknown values retain
// their raw spelling while remaining unavailable to tier/approximation rules.
func (table Table) Resolve(raw string) Value {
	result := Value{EffortRaw: raw}
	key := normalize(raw)
	if key == "" {
		return result
	}
	for _, exact := range table.Exact {
		if key == normalize(exact) {
			return exactValue(raw, normalize(exact))
		}
	}
	for alias, tier := range table.Aliases {
		if key == normalize(alias) {
			return exactValue(raw, normalize(tier))
		}
	}
	return result
}

func exactValue(raw, tier string) Value {
	approx := false
	return Value{Effort: tier, EffortRaw: raw, EffortApprox: &approx}
}

func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
