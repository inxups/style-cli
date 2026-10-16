package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
)

func installedSnapshot(home string, item lockPackage) (map[string][]byte, error) {
	files, single, _, err := snapshotPackage(lockedPackagePath(home, item))
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", item.Name, err)
	}
	if single != (item.Source.Kind == "file") {
		return nil, fmt.Errorf("package %s changed source kind", item.Name)
	}
	// Managed single-file storage uses the package name, while source paths and
	// lock hashes retain the original filename.
	if single {
		for _, data := range files {
			return map[string][]byte{filepath.Base(item.Source.Path): data}, nil
		}
	}
	return files, nil
}

func (s *Service) parseInstalledSnapshot(ctx context.Context, home string, item lockPackage, files map[string][]byte) (installPackage, error) {
	if err := checkContext(ctx); err != nil {
		return installPackage{}, err
	}
	name := ""
	if item.Source.Kind == "file" {
		name = filepath.Base(item.Source.Path)
	}
	pkg, err := spec.LoadSnapshot(lockedPackagePath(home, item), files, name)
	if err != nil {
		return installPackage{}, fmt.Errorf("load %s: %w", item.Name, err)
	}
	if diagnostics := s.validateSpec(pkg); hasSpecErrors(diagnostics) {
		return installPackage{}, &ValidationError{Diagnostics: convertDiagnostics(diagnostics)}
	}
	if pkg.Metadata["name"] != item.Name {
		return installPackage{}, &ConflictError{Message: fmt.Sprintf("package %s cannot change its name; uninstall and install the new package", item.Name)}
	}
	if err := validateRuntimeRequirement(pkg.Metadata["requires"]); err != nil {
		return installPackage{}, err
	}
	stored, err := readSettingValues(home, item.Name)
	if err != nil {
		return installPackage{}, err
	}
	// A removed setting is no longer supplied. New settings still require a
	// default or a separately configured value; approval never invents one.
	supplied := make(map[string]string)
	declarations, _ := pkg.Metadata["settings"].([]any)
	for _, raw := range declarations {
		fields, _ := raw.(map[string]any)
		key, _ := fields["name"].(string)
		if value, ok := stored[key]; ok {
			supplied[key] = value
		}
	}
	settings, names, err := collectSettings(pkg, supplied)
	if err != nil {
		return installPackage{}, err
	}
	resolved, err := spec.Merge(pkg)
	if err != nil {
		return installPackage{}, err
	}
	resolved, err = resolvedWithSettings(resolved, settings)
	if err != nil {
		return installPackage{}, err
	}
	item.Files, _ = hashPackageFiles(files)
	item.Version, _ = pkg.Metadata["version"].(string)
	item.Settings = names
	item.Replace = packageUsesReplace(resolved)
	return installPackage{lock: item, pkg: pkg, resolved: resolved, settings: settings,
		report: make(map[adapter.ProviderID]plan.Report), tokens: make(map[adapter.ProviderID]tokens.Report)}, nil
}
