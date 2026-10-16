// Package app coordinates STYLE use cases without depending on command parsing.
package app

import (
	"context"
	"fmt"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
)

// Service holds the explicitly registered adapters used by planning.
type Service struct {
	definitions map[adapter.ProviderID]adapter.Definition
	native      spec.NativeCatalog
}

// New constructs a service from an explicit adapter catalog.
func New(definitions ...adapter.Definition) (*Service, error) {
	registry, err := adapter.NewRegistry(definitions...)
	if err != nil {
		return nil, err
	}
	service := &Service{definitions: make(map[adapter.ProviderID]adapter.Definition), native: registry.NativeCatalog()}
	for _, provider := range registry.Providers() {
		definition, ok := registry.Lookup(provider)
		if !ok {
			return nil, fmt.Errorf("app: adapter %q disappeared from registry", provider)
		}
		service.definitions[provider] = definition
	}
	return service, nil
}

// validateSpec runs schema validation plus the registry-dependent native
// checks. Every path that accepts a package must use it instead of
// spec.Validate alone.
func (s *Service) validateSpec(pkg *spec.Package) []spec.Diagnostic {
	diagnostics := spec.Validate(pkg)
	return append(diagnostics, spec.ValidateNative(pkg, s.native)...)
}

// CheckContext returns the context error before a use case starts filesystem
// work or invokes a provider detector.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("app: context is required")
	}
	return ctx.Err()
}
