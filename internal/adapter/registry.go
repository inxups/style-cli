package adapter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
)

// Registry is an immutable, explicitly constructed set of provider
// definitions. It has no package-global registrations and is safe for
// concurrent lookups after construction.
type Registry struct {
	providers map[ProviderID]Definition
}

// NewRegistry validates and copies provider definitions. Route order is
// preserved; provider enumeration is sorted by canonical ID.
func NewRegistry(definitions ...Definition) (*Registry, error) {
	registry := &Registry{providers: make(map[ProviderID]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := registry.providers[definition.ID]; exists {
			return nil, fmt.Errorf("adapter: duplicate provider %q", definition.ID)
		}
		registry.providers[definition.ID] = cloneDefinition(definition)
	}
	return registry, nil
}

// Lookup returns a copy of the definition for provider, if registered.
func (r *Registry) Lookup(provider ProviderID) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	definition, ok := r.providers[provider]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(definition), true
}

// Providers returns canonical provider IDs in stable lexical order.
func (r *Registry) Providers() []ProviderID {
	if r == nil {
		return nil
	}
	providers := make([]ProviderID, 0, len(r.providers))
	for provider := range r.providers {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i] < providers[j] })
	return providers
}

// NativeCatalog returns the native hook surface of every provider that
// accepts native_hooks declarations, keyed by provider ID (rules §3.10).
func (r *Registry) NativeCatalog() spec.NativeCatalog {
	catalog := make(spec.NativeCatalog)
	if r == nil {
		return catalog
	}
	for provider, definition := range r.providers {
		if definition.Native != nil {
			catalog[string(provider)] = definition.Native
		}
	}
	return catalog
}

// ValidationCatalog returns independent containers for all registered names
// and their native subset. Capability instances are immutable adapter facts.
func (r *Registry) ValidationCatalog() spec.ValidationCatalog {
	providers := r.Providers()
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, string(provider))
	}
	return spec.ValidationCatalog{Harnesses: names, Native: r.NativeCatalog()}
}

// BuildPlanAdapter adapts a provider's richer planning facts to core/plan.
func BuildPlanAdapter(provider ProviderID, facts PlanningFacts) (plan.Adapter, error) {
	if strings.TrimSpace(string(provider)) == "" {
		return nil, fmt.Errorf("adapter: provider ID must not be empty")
	}
	if facts == nil {
		return nil, fmt.Errorf("adapter: planning facts are required for %q", provider)
	}
	return planAdapter{provider: provider, facts: facts}, nil
}

func validateDefinition(definition Definition) error {
	if strings.TrimSpace(string(definition.ID)) == "" {
		return fmt.Errorf("adapter: provider ID must not be empty")
	}
	if definition.Detector == nil {
		return fmt.Errorf("adapter: detector is required for %q", definition.ID)
	}
	if definition.Facts == nil {
		return fmt.Errorf("adapter: planning facts are required for %q", definition.ID)
	}
	if definition.StaticPositions == nil {
		return fmt.Errorf("adapter: static position catalog is required for %q", definition.ID)
	}
	if definition.Limits.Input.Value <= 0 {
		return fmt.Errorf("adapter: input limit must be positive for %q", definition.ID)
	}
	if definition.Limits.Input.Unit != tokens.TokensUnit && definition.Limits.Input.Unit != tokens.CharsUnit {
		return fmt.Errorf("adapter: input limit unit must be tokens or chars for %q", definition.ID)
	}
	switch definition.Limits.Overflow {
	case OverflowFilePreview, OverflowSilentDrop, OverflowReject, OverflowUnknown:
	default:
		return fmt.Errorf("adapter: overflow behavior must be declared for %q", definition.ID)
	}
	if definition.Compact.Event == "" && (definition.Compact.SessionSource != "" || definition.Compact.Deferred) {
		return fmt.Errorf("adapter: unsupported compact signal cannot have source or deferred metadata for %q", definition.ID)
	}
	routeNames := make(map[string]bool, len(definition.Routes))
	for _, route := range definition.Routes {
		if route == nil {
			return fmt.Errorf("adapter: nil route for %q", definition.ID)
		}
		name := strings.TrimSpace(route.Name())
		if name == "" {
			return fmt.Errorf("adapter: route name must not be empty for %q", definition.ID)
		}
		if routeNames[name] {
			return fmt.Errorf("adapter: duplicate route %q for %q", name, definition.ID)
		}
		routeNames[name] = true
	}
	return nil
}

func cloneDefinition(definition Definition) Definition {
	definition.Routes = append([]Route(nil), definition.Routes...)
	return definition
}

type planAdapter struct {
	provider ProviderID
	facts    PlanningFacts
}

func (adapter planAdapter) Name() string { return string(adapter.provider) }

func (adapter planAdapter) Supports(point string, capability plan.Capability) bool {
	return adapter.facts.Capability(point, capability).Available
}

func (adapter planAdapter) DataSource(point, field string) bool {
	return adapter.facts.DataSource(point, field).Available
}

func (adapter planAdapter) BlindSpot(rule spec.Rule) (string, bool) {
	return adapter.facts.BlindSpot(rule)
}

// NativeHook implements plan.NativeAdapter for providers whose facts
// support native hooks; other providers report the mapping as unwritable.
func (adapter planAdapter) NativeHook(rule spec.NativeMapping) (plan.NativeEntry, error) {
	native, ok := adapter.facts.(NativePlanningFacts)
	if !ok {
		return plan.NativeEntry{}, fmt.Errorf("adapter %s does not support native hooks", adapter.provider)
	}
	return native.NativeHook(rule)
}

func (adapter planAdapter) StaticLocation(mode string) (string, bool) {
	location, ok := adapter.facts.StaticLocation(mode)
	if !ok || !location.Supported {
		return "", false
	}
	return location.Name, location.Name != ""
}
