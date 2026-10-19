package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
)

// harnessSelection is one parsed --harness list.
type harnessSelection struct {
	// Auto is set when no harness was named or auto was among the values.
	Auto bool
	// Named lists explicit harnesses in first-seen order without duplicates.
	Named []adapter.ProviderID
}

func (h harnessSelection) Has(provider adapter.ProviderID) bool {
	return slices.Contains(h.Named, provider)
}

// harnessName canonicalizes one harness name: any case, surrounding space and
// the claude alias.
func harnessName(value string) adapter.ProviderID {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "claude" {
		value = "claude-code"
	}
	return adapter.ProviderID(value)
}

// splitHarnesses expands repeated and comma-separated harness values.
func splitHarnesses(values []string) []adapter.ProviderID {
	var names []adapter.ProviderID
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			names = append(names, harnessName(value))
		}
	}
	return names
}

// parseHarnesses is the only --harness parser. Every command accepts
// repeatable values, comma lists, any case, the claude alias and auto; auto
// combines with named harnesses. Empty or unknown names are usage errors.
func (s *Service) parseHarnesses(values []string) (harnessSelection, error) {
	selection := harnessSelection{Auto: len(values) == 0}
	for _, name := range splitHarnesses(values) {
		switch _, known := s.definitions[name]; {
		case name == "":
			return harnessSelection{}, &UsageError{Message: fmt.Sprintf("empty harness in --harness %q", strings.Join(values, ","))}
		case name == "auto":
			selection.Auto = true
		case !known:
			return harnessSelection{}, &UsageError{Message: fmt.Sprintf("unknown harness %q", name)}
		case !selection.Has(name):
			selection.Named = append(selection.Named, name)
		}
	}
	return selection, nil
}

// singleHarness parses a --harness list that must name exactly one harness,
// for commands that read one harness's payload.
func (s *Service) singleHarness(values []string) (string, error) {
	selection, err := s.parseHarnesses(values)
	if err != nil {
		return "", err
	}
	if selection.Auto || len(selection.Named) != 1 {
		return "", &UsageError{Message: "select exactly one harness"}
	}
	return string(selection.Named[0]), nil
}
