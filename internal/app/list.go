package app

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/source"
)

// ListRequest selects the local installation store without probing providers.
type ListRequest struct{ Home string }

type InstalledPackage struct {
	Git          *source.Git `json:"git,omitempty"`
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Sequence     int         `json:"seq"`
	Source       string      `json:"source"`
	SourceKind   string      `json:"source_kind"`
	Link         string      `json:"link"`
	HashMatches  bool        `json:"hash_matches"`
	ChangedFiles []string    `json:"changed_files"`
	Error        string      `json:"error,omitempty"`
	NativeCount  int         `json:"native_count"`
}

type ListResult struct {
	Packages []InstalledPackage `json:"packages"`
}

// List compares the actual installed bytes with the approved lock hashes.
// Missing or unreadable sources remain visible rather than hiding packages.
func (s *Service) List(ctx context.Context, request ListRequest) (ListResult, error) {
	if err := checkContext(ctx); err != nil {
		return ListResult{}, err
	}
	home, err := resolveStyleHome(request.Home)
	if err != nil {
		return ListResult{}, err
	}
	lock, err := loadLock(home)
	if err != nil {
		return ListResult{}, err
	}
	return listLocked(ctx, home, lock)
}

func listLocked(ctx context.Context, home string, lock lockDocument) (ListResult, error) {
	result := ListResult{Packages: []InstalledPackage{}}
	for _, pkg := range lock.Packages {
		if err := ctx.Err(); err != nil {
			return ListResult{}, err
		}
		item := InstalledPackage{Name: pkg.Name, Version: pkg.Version, Sequence: pkg.Seq,
			Source: pkg.Source.Path, SourceKind: pkg.Source.Kind, Link: "no", ChangedFiles: []string{}}
		if pkg.Source.Git != nil {
			git := *pkg.Source.Git
			item.Git = &git
		}
		if pkg.Link {
			item.Link = "dev"
		}
		files, err := installedSnapshot(home, pkg)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.ChangedFiles = filesChanged(pkg, files)
			item.HashMatches = len(item.ChangedFiles) == 0
		}
		baseline, available, baselineErr := loadApproval(home, pkg)
		if baselineErr != nil {
			item.Error = baselineErr.Error()
		} else if available && baseline.Native != nil {
			item.NativeCount = len(baseline.Native)
		} else {
			if available {
				files = baseline.Files
			}
			if resolved, err := loadNativeSource(pkg, files); err == nil {
				for _, rule := range resolved.Rules {
					if rule.Effective["target"] != "native" {
						continue
					}
					for _, native := range spec.NativeRules(rule) {
						if _, ok := lock.Harnesses[native.Harness]; ok {
							item.NativeCount++
						}
					}
				}
			}
		}
		result.Packages = append(result.Packages, item)
	}
	sort.Slice(result.Packages, func(i, j int) bool {
		left, right := result.Packages[i], result.Packages[j]
		return left.Sequence < right.Sequence || left.Sequence == right.Sequence && left.Name < right.Name
	})
	return result, nil
}

func lockedPackagePath(home string, pkg lockPackage) string {
	if pkg.Link {
		return pkg.Source.Path
	}
	if pkg.Source.Kind == "file" {
		return filepath.Join(home, "packages", pkg.Name+".style.md")
	}
	return filepath.Join(home, "packages", pkg.Name)
}
