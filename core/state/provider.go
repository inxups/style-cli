package state

import (
	"fmt"
	"regexp"
)

var modelProviderID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// validateProviderState keeps the optional provider cache bounded and prevents
// arbitrary config or endpoint strings from being replayed as provenance.
func validateProviderState(session Session) error {
	if !session.ProviderResolved {
		if session.ModelProvider != "" || session.ProviderSource != "" {
			return fmt.Errorf("state: incomplete provider reading")
		}
		return nil
	}
	if session.ModelProvider != "" && (session.ModelProvider == "unknown" || !modelProviderID.MatchString(session.ModelProvider)) {
		return fmt.Errorf("state: invalid provider identifier")
	}
	switch session.ProviderSource {
	case "provided", "offline-unavailable", "codex-launch", "codex-profile", "codex-user", "codex-default", "codex-provider-ancestry-unavailable", "codex-provider-invalid-launch", "codex-provider-unreplayable", "codex-provider-version-unavailable", "codex-provider-config-unavailable", "claude-default", "claude-environment", "claude-provider-managed-or-endpoint", "claude-provider-unsupported-route", "claude-provider-conflicting-selectors", "claude-provider-invalid-selector":
		return nil
	}
	return fmt.Errorf("state: invalid provider provenance")
}
