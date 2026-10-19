package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks/install"
	"gopkg.in/yaml.v3"
)

type UninstallRequest struct {
	Name string
	All  bool
	Home string
	Yes  bool
}

type UninstallResult struct {
	StaticMigrations []adapter.StaticMigration `json:"static_migrations,omitempty"`
	Removed          []string                  `json:"removed"`
	Remaining        []string                  `json:"remaining"`
	Routes           []string                  `json:"routes"`
	Changed          bool                      `json:"changed"`
}

// Uninstall removes selected packages and rebuilds each managed adapter route
// from the remaining lock entries. Provider files are changed through their
// route changesets, which retain the shared agenthooks owner semantics.
func (s *Service) Uninstall(ctx context.Context, request UninstallRequest) (final UninstallResult, runErr error) {
	if err := checkContext(ctx); err != nil {
		return UninstallResult{}, err
	}
	if !request.Yes {
		return UninstallResult{}, &RejectedError{Message: "uninstall requires confirmation"}
	}
	if request.All == (strings.TrimSpace(request.Name) != "") {
		return UninstallResult{}, &UsageError{Message: "specify one package name or --all"}
	}
	home, err := s.resolveStyleHome(request.Home)
	if err != nil {
		return UninstallResult{}, err
	}
	inputs, err := s.captureManagement(home)
	if err != nil {
		return UninstallResult{}, err
	}
	lock, err := s.loadLock(home)
	if err != nil {
		return UninstallResult{}, err
	}
	if err = inputs.links(lock); err != nil {
		return UninstallResult{}, err
	}
	removeSet := make(map[string]bool)
	for _, item := range lock.Packages {
		if request.All || item.Name == request.Name {
			removeSet[item.Name] = true
		}
	}
	if len(removeSet) == 0 {
		return UninstallResult{}, &ConflictError{Message: "no matching installed package"}
	}
	result := UninstallResult{Removed: make([]string, 0, len(removeSet)), Remaining: make([]string, 0), Routes: make([]string, 0)}
	remainingLock := lockDocument{Format: lock.Format, Project: lock.Project, Packages: make([]lockPackage, 0, len(lock.Packages)), Harnesses: cloneHarnesses(lock.Harnesses)}
	removedPackages := make([]lockPackage, 0, len(removeSet))
	for _, item := range lock.Packages {
		if removeSet[item.Name] {
			removedPackages = append(removedPackages, item)
			result.Removed = append(result.Removed, item.Name)
			continue
		}
		remainingLock.Packages = append(remainingLock.Packages, item)
		result.Remaining = append(result.Remaining, item.Name)
	}
	sort.Strings(result.Removed)
	sort.Slice(result.Remaining, func(i, j int) bool {
		left, right := packageSequence(remainingLock, result.Remaining[i]), packageSequence(remainingLock, result.Remaining[j])
		if left != right {
			return left < right
		}
		return result.Remaining[i] < result.Remaining[j]
	})

	adapters, err := s.lockedAdapters(ctx, home, lock)
	if err != nil {
		return UninstallResult{}, err
	}
	if err = inputs.routes(adapters); err != nil {
		return UninstallResult{}, err
	}
	packages, err := s.loadInstalledPackages(ctx, home, remainingLock)
	if err != nil {
		return UninstallResult{}, err
	}
	mapping, err := s.captureEffortMapping(home, inputs)
	if err != nil {
		return UninstallResult{}, err
	}
	compilation, err := s.compileInstalled(ctx, packages, adapters, "", mapping)
	if err != nil {
		return UninstallResult{}, err
	}
	changes := make([]install.PlannedFile, 0)
	if len(remainingLock.Packages) == 0 {
		for _, target := range adapters {
			removed, err := target.route.Remove(ctx, adapter.Owner("style"))
			if err != nil {
				return UninstallResult{}, fmt.Errorf("plan route removal %s: %w", target.route.Name(), err)
			}
			changes = append(changes, routePlannedFiles(removed)...)
			result.StaticMigrations = append(result.StaticMigrations, removed.Migrations...)
			result.Routes = append(result.Routes, string(target.provider)+"="+target.route.Name())
			delete(remainingLock.Harnesses, string(target.provider))
		}
		for _, path := range []string{filepath.Join(home, "build", "index.json"), filepath.Join(home, "build", "explain.json"), filepath.Join(home, "style.lock.yaml")} {
			removals, err := plannedRemoval(path)
			if err != nil {
				return UninstallResult{}, err
			}
			changes = append(changes, removals...)
		}
	} else {
		for index := range adapters {
			target := &adapters[index]
			harness := lock.Harnesses[string(target.provider)]
			target.hooks = compilation.hooksForProvider(target.provider)
			if harness.FailClosed {
				for hookIndex := range target.hooks {
					if target.hooks[hookIndex].Command != "" {
						continue
					}
					extra := cloneExtra(target.hooks[hookIndex].Extra)
					if extra == nil {
						extra = make(map[string]any)
					}
					extra["fail_closed"] = true
					target.hooks[hookIndex].Extra = extra
				}
			}
			blocks := staticBlocksForProvider(compilation.staticBodies, target.provider)
			planned, err := target.route.Plan(ctx, target.hooks, blocks)
			if err != nil {
				return UninstallResult{}, fmt.Errorf("plan route %s: %w", target.route.Name(), err)
			}
			changes = append(changes, routePlannedFiles(planned)...)
			remainingLock.Harnesses[string(target.provider)] = lockHarness{Route: target.route.Name(), Version: harness.Version, ConfigDir: harness.ConfigDir,
				Surface: hashSurface(target.hooks), Rendered: hashChangeset(planned), Trust: trustState(target.route.Trust()), FailClosed: harness.FailClosed}
			remainingLock.Harnesses[string(target.provider)] = withStaticState(remainingLock.Harnesses[string(target.provider)], planned)
			result.StaticMigrations = append(result.StaticMigrations, planned.Migrations...)
			result.Routes = append(result.Routes, string(target.provider)+"="+target.route.Name())
		}
		for i, item := range packages {
			approval, err := approvalBytes(compilation, item.lock.Name, item.pkg.Files, item.lock.Source.Git)
			if err != nil {
				return UninstallResult{}, err
			}
			remainingLock.Packages[i].Approval = hashBytes(approval)
			approvalChanges, err := planPrivateFile(approvalPath(home, item.lock.Name), approval)
			if err != nil {
				return UninstallResult{}, err
			}
			changes = append(changes, approvalChanges...)
		}
		changes = append(changes, plannedFile(filepath.Join(home, "build"), "index.json", compilation.indexBytes, 0o600)...)
		changes = append(changes, plannedFile(filepath.Join(home, "build"), "explain.json", compilation.provenanceBytes, 0o600)...)
	}
	for _, item := range removedPackages {
		approvalRemovals, err := plannedRemoval(approvalPath(home, item.Name))
		if err != nil {
			return UninstallResult{}, err
		}
		changes = append(changes, approvalRemovals...)
		removals, err := plannedRemoval(filepath.Join(home, "settings", item.Name+".yaml"))
		if err != nil {
			return UninstallResult{}, err
		}
		changes = append(changes, removals...)
	}
	changes, err = s.projectFiles(home, lock, &remainingLock, adapters, compilation, changes, inputs)
	if err != nil {
		return result, err
	}
	if len(remainingLock.Packages) > 0 {
		lockBytes, err := yaml.Marshal(remainingLock)
		if err != nil {
			return result, err
		}
		changes = append(changes, plannedFile(home, "style.lock.yaml", lockBytes, 0o600)...)
	}
	changes, err = uniquePlannedFiles(changes)
	if err != nil {
		return UninstallResult{}, err
	}
	if err = inputs.planned(changes); err != nil {
		return result, err
	}
	guard, err := acquireInputs(ctx, inputs, home)
	if err != nil {
		return result, err
	}
	defer func() { runErr = errors.Join(runErr, guard.close()) }()
	if err = inputs.verify(); err != nil {
		return result, err
	}
	var swaps []*packageSwap
	for _, item := range removedPackages {
		if item.Link {
			continue
		}
		path := filepath.Join(home, "packages", item.Name)
		if item.Source.Kind == "file" {
			path += ".style.md"
		}
		swap, err := stagePackageRemoval(path)
		if err != nil {
			return UninstallResult{}, err
		}
		if swap != nil {
			swaps = append(swaps, swap)
		}
	}
	committed := false
	defer func() {
		if !committed {
			for index := len(swaps) - 1; index >= 0; index-- {
				runErr = errors.Join(runErr, swaps[index].rollback())
			}
		}
	}()
	for _, swap := range swaps {
		if err := swap.apply(); err != nil {
			return UninstallResult{}, fmt.Errorf("stage package removal: %w", err)
		}
	}
	if err := install.ApplyFiles(ctx, changes); err != nil {
		return UninstallResult{}, fmt.Errorf("apply uninstall changes: %w", err)
	}
	for _, swap := range swaps {
		swap.commit()
	}
	committed = true
	result.Changed = true
	sort.Strings(result.Routes)
	return result, nil
}

func (s *Service) lockedAdapters(ctx context.Context, home string, lock lockDocument) ([]installAdapter, error) {
	providers := make([]string, 0, len(lock.Harnesses))
	for provider := range lock.Harnesses {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		left, right := providerOrder(adapter.ProviderID(providers[i])), providerOrder(adapter.ProviderID(providers[j]))
		if left != right {
			return left < right
		}
		return providers[i] < providers[j]
	})
	result := make([]installAdapter, 0, len(providers))
	for _, raw := range providers {
		provider := adapter.ProviderID(raw)
		definition, ok := s.definitions[provider]
		if !ok {
			return nil, &ConflictError{Message: fmt.Sprintf("installed provider %q is not registered", provider)}
		}
		name := lock.Harnesses[raw].Route
		route := routeByName(definition, name)
		if route == nil {
			return nil, &ConflictError{Message: fmt.Sprintf("installed route %q for %s is unavailable", name, provider)}
		}
		installation := adapter.Installation{Provider: provider, Version: lock.Harnesses[raw].Version, ConfigDir: lock.Harnesses[raw].ConfigDir}
		route = bindRoute(route, installation, home, s.staticState(lock, provider))
		result = append(result, installAdapter{provider: provider, definition: definition, installation: installation, route: route})
	}
	return result, nil
}

func cloneHarnesses(values map[string]lockHarness) map[string]lockHarness {
	result := make(map[string]lockHarness, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func packageSequence(lock lockDocument, name string) int {
	item, _ := findLockedPackage(lock, name)
	return item.Seq
}

func plannedRemoval(path string) ([]install.PlannedFile, error) {
	before, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return []install.PlannedFile{{Path: path, Root: filepath.Dir(path), Before: before, Mode: mode, Remove: true}}, nil
}
