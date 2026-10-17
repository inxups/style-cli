package compile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
)

// EffortMapping is a normalized, explicit user interpretation of native raw values.
// Adapter code resolves native facts; core only carries canonical configuration.
type EffortMapping map[string]map[string]string

var effortMappingID = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

func (m EffortMapping) Validate() error {
	for harness, rows := range m {
		if !effortMappingID.MatchString(harness) || len(rows) == 0 || len(rows) > 128 {
			return fmt.Errorf("compile: invalid effort mapping harness or size")
		}
		for raw, tier := range rows {
			if !effortMappingID.MatchString(raw) {
				return fmt.Errorf("compile: invalid normalized effort mapping key")
			}
			switch tier {
			case "none", "minimal", "low", "medium", "high", "xhigh", "max", "open":
			default:
				return fmt.Errorf("compile: invalid effort mapping tier")
			}
		}
	}
	return nil
}
func (m EffortMapping) Clone() EffortMapping {
	if len(m) == 0 {
		return nil
	}
	out := EffortMapping{}
	for h, rows := range m {
		out[h] = map[string]string{}
		for k, v := range rows {
			out[h][k] = v
		}
	}
	return out
}

// SHA256 fingerprints one harness without disclosing its raw keys.
func (m EffortMapping) SHA256(harness string) string {
	if len(m[harness]) == 0 {
		return ""
	}
	data, _ := json.Marshal(m[harness])
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
