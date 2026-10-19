package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/inxups/style-cli/internal/source"
)

// SourceReport adds provenance to Git previews without changing local reports.
type SourceReport struct {
	Kind       string     `json:"kind"`
	Path       string     `json:"path"`
	Git        source.Git `json:"git"`
	CacheHit   bool       `json:"cache_hit"`
	GitVersion string     `json:"git_version,omitempty"`
}

func (s *Service) acquireGit(ctx context.Context, request source.Request) (source.Snapshot, *SourceReport, error) {
	home, err := s.resolveStyleHome(request.Home)
	if err != nil {
		return source.Snapshot{}, nil, err
	}
	request.Home = home
	// Reject malformed addresses before any acquirer can start Git.
	if _, err := source.Parse(request); err != nil {
		return source.Snapshot{}, nil, err
	}
	resolver := s.sources
	if resolver == nil {
		resolver = &source.Resolver{}
	}
	snapshot, err := resolver.Resolve(ctx, request)
	if err != nil {
		return source.Snapshot{}, nil, err
	}
	return snapshot, &SourceReport{Kind: snapshot.Kind, Path: snapshot.Path, Git: snapshot.Git, CacheHit: snapshot.CacheHit, GitVersion: snapshot.GitVersion}, nil
}

// sourceArgument checks a source as the command line spells it: a malformed
// Git address is a usage error, before any home, cache or Git is touched.
func sourceArgument(value string) error {
	if !source.Remote(value) {
		return nil
	}
	_, err := source.Parse(source.Request{Source: value})
	return err
}

func localSourceOptions(ref, p string) error {
	if ref != "" || p != "" {
		return &source.Error{Code: "source-invalid", Message: "ref and package path require a git+https source"}
	}
	return nil
}

func writeSource(out io.Writer, report *SourceReport) {
	if report == nil {
		return
	}
	fmt.Fprintf(out, "Source: %q ref=%q commit=%s path=%q (%s; cache_hit=%t)\n", report.Git.URL, report.Git.Ref, report.Git.Commit, report.Path, report.Kind, report.CacheHit)
	fmt.Fprintf(out, "  object: %s:%s; content: %s\n", report.Git.ObjectFormat, report.Git.Object, report.Git.ContentSHA256)
}

func sameGitSource(existing lockPackage, git *source.Git, kind, p string) bool {
	if existing.Source.Git == nil && git == nil {
		return true
	}
	return existing.Source.Git != nil && git != nil && *existing.Source.Git == *git && existing.Source.Kind == kind && existing.Source.Path == p
}

func checkGitSnapshot(item lockPackage, files map[string][]byte) error {
	if item.Source.Git != nil {
		hashes, aggregate := source.Hashes(files)
		if !reflectStringMap(hashes, item.Files) || aggregate != item.Source.Git.ContentSHA256 {
			return &source.Error{Code: "git-snapshot-changed", Message: fmt.Sprintf("Git package %q differs from its locked export; restore locked bytes or use a separate local/link source", item.Name)}
		}
	}
	return nil
}

func snapshotName(kind, p string) string {
	if kind == "file" {
		return filepath.Base(p)
	}
	return ""
}
