package app

import (
	"fmt"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
)

// A missing detected version cannot establish compatibility with an explicit
// requirement. Packages without a requirement retain the offline behavior.
func harnessRequirement(metadata map[string]any, provider adapter.ProviderID, version string) *Diagnostic {
	requires, _ := metadata["requires"].(map[string]any)
	harnesses, _ := requires["harness"].(map[string]any)
	constraint, _ := harnesses[string(provider)].(string)
	if constraint == "" || strings.TrimSpace(version) != "" && versionSatisfies(version, constraint) {
		return nil
	}
	if strings.TrimSpace(version) == "" {
		version = "unknown"
	}
	return &Diagnostic{Code: "harness-version", Provider: string(provider), Severity: "warning",
		Message: fmt.Sprintf("skipping %s for package %s: version %s does not satisfy requires.harness.%s %s", provider, metadata["name"], version, provider, constraint)}
}
