package state

import (
	"fmt"
	"regexp"
)

var effortMappingSHA = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func validateEffortMappingState(session Session) error {
	if session.EffortMappingSHA256 == "" {
		if session.EffortMappingSource != "" {
			return fmt.Errorf("state: incomplete effort mapping provenance")
		}
		return nil
	}
	if !effortMappingSHA.MatchString(session.EffortMappingSHA256) {
		return fmt.Errorf("state: invalid effort mapping digest")
	}
	switch session.EffortMappingSource {
	case "user", "builtin", "unmapped":
		return nil
	}
	return fmt.Errorf("state: invalid effort mapping provenance")
}
