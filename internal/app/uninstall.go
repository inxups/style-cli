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
	Removed   []string `json:"removed"`
	Remaining []string `json:"remaining"`
	Routes    []string `json:"routes"`
	Changed   bool     `json:"changed"`
}

// Uninstall removes selected packages and rebuilds each managed adapter route
// from the remaining lock entries. Provider files are changed through their
// route changesets, which retain the shared agenthooks owner semantics.
func (s *Service) Uninstall(ctx context.Context, request UninstallRequest) (UninstallResult, error) {
	if err := checkContext(ctx); err != nil {
		return UninstallResult{}, err
	}
	if !request.Yes {
		return UninstallResult{}, &UsageError{Message: "uninstall requires confirmation; pass --yes in non-interactive mode"}
	}
	if request.All == (strings.TrimSpace(request.Name) != "") {
		return UninstallResult{}, &UsageError{Message: "specify one package name or --all"}
	}
	home, err := resolveStyleHome(request.Home)
	if err != nil {
		return UninstallResult{}, err
	}
	lock, err := loadLock(home)
	if err != nil {
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
	remainingLock := lockDocument{Format: lockFormat, Packages: make([]lockPackage, 0, len(lock.Packages)), Harnesses: cloneHarnesses(lock.Harnesses)}
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

	adapters, err := s.lockedAdapters(ctx, lock)
	if err != nil {
		return UninstallResult{}, err
	}
	packages, err := s.loadInstalledPackages(ctx, home, remainingLock)
	if err != nil {
		return UninstallResult{}, err
	}
	compilation, err := s.compileInstalled(ctx, packages, adapters, "")
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
			result.Routes = append(result.Routes, string(target.provider)+"="+target.route.Name())
			delete(remainingLock.Harnesses, string(target.provider))
		}
		for _, path := range []string{filepath.Join(home, "build", "index.json"), filepath.Join(home, "style.lock.yaml")} {
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
			target.hooks = hooksForProvider(target.provider, compilation.index)
			if harness.FailClosed {
				for hookIndex := range target.hooks {
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
			result.Routes = append(result.Routes, string(target.provider)+"="+target.route.Name())
		}
		lockBytes, err := yaml.Marshal(remainingLock)
		if err != nil {
			return UninstallResult{}, err
		}
		changes = append(changes, plannedFile(home, "style.lock.yaml", lockBytes, 0o600)...)
		changes = append(changes, plannedFile(filepath.Join(home, "build"), "index.json", compilation.indexBytes, 0o600)...)
	}
	for _, item := range removedPackages {
		removals, err := plannedRemoval(filepath.Join(home, "settings", item.Name+".yaml"))
		if err != nil {
			return UninstallResult{}, err
		}
		changes = append(changes, removals...)
	}
	changes, err = uniquePlannedFiles(changes)
	if err != nil {
		return UninstallResult{}, err
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
				swaps[index].rollback()
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

func (s *Service) lockedAdapters(ctx context.Context, lock lockDocument) ([]installAdapter, error) {
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
		if binder, ok := route.(adapter.InstallationBinder); ok {
			route = binder.BindInstallation(installation)
		}
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
