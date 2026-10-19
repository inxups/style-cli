package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/source"
)

type ProjectPackagePreview struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Source   string        `json:"source"`
	Git      *SourceReport `json:"git_source,omitempty"`
	Settings []string      `json:"settings,omitempty"`
	Replace  bool          `json:"replace"`
}

// Extra sources make a multi-package clone reviewable as one candidate and one
// transaction. Their settings use explicit package-qualified names.
func (s *Service) captureProjectPackages(ctx context.Context, request InstallRequest, home string, lock lockDocument, adapters []installAdapter, first lockPackage) ([]installPackage, []ProjectPackagePreview, error) {
	if len(request.ProjectSources) == 0 {
		if len(request.ProjectSettings) > 0 {
			return nil, nil, &UsageError{Message: "--project-set requires --project-source"}
		}
		return nil, nil, nil
	}
	if s.project == nil || request.Link || len(request.ProjectSources) > 63 {
		return nil, nil, &UsageError{Message: "--project-source requires --project, at most 64 sources, and copied packages"}
	}
	if len(lock.Packages) > 0 {
		return nil, nil, &RejectedError{Message: "--project-source requires a fresh project installation"}
	}
	seen := map[string]bool{first.Name: true}
	replace := first.Replace
	result := []installPackage{}
	previews := []ProjectPackagePreview{}
	usedSettings := map[string]bool{}
	for i, path := range request.ProjectSources {
		if err := checkContext(ctx); err != nil {
			return nil, nil, err
		}
		var files map[string][]byte
		var single bool
		var origin string
		var git *source.Git
		var report *SourceReport
		var err error
		if source.Remote(path) {
			var snapshot source.Snapshot
			snapshot, report, err = s.acquireGit(ctx, source.Request{Source: path, Ref: request.Ref, Path: request.Path, Home: home})
			files, single, origin, git = snapshot.Files, snapshot.Kind == "file", snapshot.Path, &snapshot.Git
		} else {
			err = localSourceOptions(request.Ref, request.Path)
			if err == nil {
				files, single, origin, err = snapshotPackage(path)
			}
		}
		if err != nil {
			return nil, nil, err
		}
		singleName := ""
		if single {
			singleName = filepath.Base(origin)
		}
		pkg, err := spec.LoadSnapshot(origin, files, singleName)
		if err != nil {
			return nil, nil, err
		}
		name, _ := pkg.Metadata["name"].(string)
		version, _ := pkg.Metadata["version"].(string)
		if !lockedNamePattern.MatchString(name) || version == "" || seen[name] {
			return nil, nil, &ConflictError{Message: "duplicate or invalid project package name"}
		}
		seen[name] = true
		if err = validateRuntimeRequirement(pkg.Metadata["requires"]); err != nil {
			return nil, nil, err
		}
		diagnostics := s.validateSpec(pkg)
		if hasSpecErrors(diagnostics) {
			return nil, nil, &ValidationError{Diagnostics: convertDiagnostics(diagnostics)}
		}
		values := map[string]string{}
		for key, value := range request.ProjectSettings {
			prefix := name + "."
			if strings.HasPrefix(key, prefix) {
				values[strings.TrimPrefix(key, prefix)] = value
				usedSettings[key] = true
			}
		}
		settings, keys, err := collectSettings(pkg, values)
		if err != nil {
			return nil, nil, err
		}
		resolved, err := spec.Merge(pkg)
		if err != nil {
			return nil, nil, err
		}
		resolved, err = resolvedWithSettings(resolved, settings)
		if err != nil {
			return nil, nil, err
		}
		isReplace := packageUsesReplace(resolved)
		if isReplace && (replace || !request.AllowReplace) {
			return nil, nil, &ConflictError{Message: "project batch replace requires one owner and --allow-replace"}
		}
		replace = replace || isReplace
		for _, target := range adapters {
			if finding := harnessRequirement(pkg.Metadata, target.provider, target.installation.Version); finding != nil {
				return nil, nil, &BlockedError{Message: fmt.Sprintf("project package %s is incompatible with %s", name, target.provider)}
			}
		}
		hashes, _ := hashPackageFiles(files)
		seq, err := s.projectSequence(name, first.Seq+i+1)
		if err != nil {
			return nil, nil, err
		}
		item := lockPackage{Name: name, Version: version, Seq: seq, Source: lockSource{Kind: sourceKind(single), Path: origin, Git: git}, Files: hashes, Settings: keys, Replace: isReplace}
		storage := filepath.Join(home, "packages", name)
		if single {
			storage += ".style.md"
		}
		if _, err = os.Lstat(storage); !os.IsNotExist(err) {
			return nil, nil, &ConflictError{Message: "project package storage already exists outside lock"}
		}
		result = append(result, installPackage{lock: item, pkg: pkg, resolved: resolved, settings: settings, source: diagnostics, report: map[adapter.ProviderID]plan.Report{}, tokens: map[adapter.ProviderID]tokens.Report{}})
		previews = append(previews, ProjectPackagePreview{Name: name, Version: version, Source: origin, Git: report, Settings: keys, Replace: isReplace})
	}
	for key := range request.ProjectSettings {
		if !usedSettings[key] {
			return nil, nil, &RejectedError{Message: "unknown package-qualified project setting " + key}
		}
	}
	return result, previews, nil
}
