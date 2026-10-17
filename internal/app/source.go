package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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

func localSourceOptions(ref, p string) error {
	if ref != "" || p != "" {
		return &source.Error{Code: "source-invalid", Message: "--ref and --path require a git+https source"}
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

func parseSourceOption(args []string, index *int, ref, p *string) (bool, error) {
	arg := args[*index]
	for _, option := range []struct {
		name  string
		value *string
	}{{"--ref", ref}, {"--path", p}} {
		if arg != option.name && !strings.HasPrefix(arg, option.name+"=") {
			continue
		}
		value := ""
		if arg == option.name {
			if *index+1 >= len(args) {
				return true, fmt.Errorf("%s requires a value", option.name)
			}
			*index++
			value = args[*index]
		} else {
			value = strings.TrimPrefix(arg, option.name+"=")
		}
		if value == "" || strings.HasPrefix(value, "--") {
			return true, fmt.Errorf("%s requires a value", option.name)
		}
		if *option.value != "" {
			return true, fmt.Errorf("%s may only be specified once", option.name)
		}
		*option.value = value
		return true, nil
	}
	return false, nil
}
