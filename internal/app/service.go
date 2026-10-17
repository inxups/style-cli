// Package app coordinates STYLE use cases without depending on command parsing.
package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/source"
)

// Service holds the explicitly registered adapters used by planning.
type Service struct {
	project     *projectScope
	sources     source.Acquirer
	probeDeps   *probeDependencies
	definitions map[adapter.ProviderID]adapter.Definition
	catalog     spec.ValidationCatalog
}

// New constructs a service from an explicit adapter catalog.
func New(definitions ...adapter.Definition) (*Service, error) {
	registry, err := adapter.NewRegistry(definitions...)
	if err != nil {
		return nil, err
	}
	service := &Service{definitions: make(map[adapter.ProviderID]adapter.Definition), catalog: registry.ValidationCatalog()}
	for _, provider := range registry.Providers() {
		definition, ok := registry.Lookup(provider)
		if !ok {
			return nil, fmt.Errorf("app: adapter %q disappeared from registry", provider)
		}
		service.definitions[provider] = definition
	}
	return service, nil
}

// validateSpec is the complete admission gate for all package paths.
func (s *Service) validateSpec(pkg *spec.Package) []spec.Diagnostic {
	return spec.ValidateWithCatalog(pkg, s.catalog)
}

// providers preserves the existing command report order while including
// every registered harness, whether or not it is installed or selected.
func (s *Service) providers() []adapter.ProviderID {
	providers := make([]adapter.ProviderID, 0, len(s.definitions))
	for provider := range s.definitions {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		left, right := providerOrder(providers[i]), providerOrder(providers[j])
		if left != right {
			return left < right
		}
		return providers[i] < providers[j]
	})
	return providers
}

// CheckContext returns the context error before a use case starts filesystem
// work or invokes a provider detector.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("app: context is required")
	}
	return ctx.Err()
}
