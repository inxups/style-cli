package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks/install"
)

type route struct {
	name           string
	dir            string
	command        string
	harnessVersion string
	facts          Facts
	storageHome    string
}

func (r *route) Name() string { return r.name }

func (r *route) BindStorage(home string) adapter.Route {
	bound := *r
	bound.storageHome = home
	return &bound
}

func (r *route) BindInstallation(installation adapter.Installation) adapter.Route {
	bound := *r
	if installation.ConfigDir != "" {
		bound.dir = installation.ConfigDir
	}
	bound.harnessVersion = installation.Version
	return &bound
}

func (r *route) Available(_ context.Context, installation adapter.Installation) (bool, string, error) {
	if installation.Provider != "" && installation.Provider != ProviderName {
		return false, "wrong provider", nil
	}
	if r.dir == "" {
		return false, "Claude Code config directory is required", nil
	}
	return true, "available", nil
}

func (r *route) Trust() adapter.TrustInfo {
	return adapter.TrustInfo{Kind: adapter.TrustWorkspace, StableKey: true, Summary: "Claude Code workspace trust controls hook execution"}
}

func (r *route) target() install.Target {
	return install.Target{Provider: ProviderName, Scope: install.ScopeUser, Dir: r.dir, HarnessVersion: r.harnessVersion}
}

func (r *route) commandLine() []string {
	argv := []string{adapter.HookCommand(r.command)}
	if r.storageHome != "" {
		argv = append(argv, "--style-home="+r.storageHome, "--harness-home="+r.dir, "--harness-version="+r.harnessVersion)
	}
	return argv
}

func (r *route) manifest(hooks []adapter.HookSpec) install.Manifest {
	return HookManifest(r.commandLine(), hooks)
}

func (r *route) InspectNative(_ context.Context, hooks []adapter.HookSpec) ([]adapter.NativeStatus, error) {
	statuses, err := install.InspectNative(r.manifest(hooks), r.target())
	result := make([]adapter.NativeStatus, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, adapter.NativeStatus{Tag: status.Tag, State: status.State, Trust: status.Trust})
	}
	return result, err
}

func (r *route) Status(ctx context.Context, installation adapter.Installation) (adapter.RouteStatus, error) {
	available, detail, err := r.Available(ctx, installation)
	if err != nil || !available {
		return adapter.RouteStatus{Available: available, Detail: detail}, err
	}
	settingsOwned, err := claudeSettingsOwned(filepath.Join(r.dir, "settings.json"))
	if err != nil {
		return adapter.RouteStatus{Available: true, Detail: err.Error()}, nil
	}
	staticOwned, err := claudeStaticOwned(filepath.Join(r.dir, "CLAUDE.md"))
	if err != nil {
		return adapter.RouteStatus{Available: true, Detail: err.Error()}, nil
	}
	return adapter.RouteStatus{
		Available: true,
		Installed: settingsOwned || staticOwned,
		// Workspace trust is external state and cannot be inferred safely from
		// the presence of settings.json.
		Trusted: false,
		Detail:  detail,
	}, nil
}

func (r *route) Plan(_ context.Context, hooks []adapter.HookSpec, blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	if len(blocks) > 0 && !r.facts.StaticAppendAvailable {
		return adapter.Changeset{}, errors.New("claude route: CLAUDE.md static slot is unavailable")
	}
	changes := make([]adapter.FileChange, 0, 2)
	settingsOwned, err := claudeSettingsOwned(filepath.Join(r.dir, "settings.json"))
	if err != nil {
		return adapter.Changeset{}, err
	}
	if len(hooks) > 0 || settingsOwned {
		planned, err := install.PlanFiles(r.manifest(hooks), r.target())
		if err != nil {
			return adapter.Changeset{}, err
		}
		for _, file := range planned {
			changes = append(changes, fileChange(file))
		}
	}
	change, changed, err := r.staticChange(func(existing []byte) ([]byte, error) {
		return mergeClaudeMD(existing, blocks)
	})
	if err != nil {
		return adapter.Changeset{}, err
	}
	if changed {
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return adapter.Changeset{Files: changes}, nil
}

func (r *route) Apply(ctx context.Context, changes adapter.Changeset) error {
	planned := make([]install.PlannedFile, 0, len(changes.Files))
	for _, change := range changes.Files {
		rel, err := filepath.Rel(r.dir, change.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("claude route: destination %q is outside route root %q", change.Path, r.dir)
		}
		planned = append(planned, install.PlannedFile{Root: r.dir, Path: change.Path, Before: change.Before, After: change.After, Mode: change.Mode, Remove: change.Remove})
	}
	return install.ApplyFiles(ctx, planned)
}

func (r *route) Remove(_ context.Context, owner adapter.Owner) (adapter.Changeset, error) {
	planned, err := install.PlanUninstallFiles(r.manifest(nil), r.target())
	if err != nil {
		return adapter.Changeset{}, err
	}
	changes := make([]adapter.FileChange, 0, len(planned)+1)
	for _, file := range planned {
		changes = append(changes, fileChange(file))
	}
	// A route owner represents the STYLE namespace. A specific package owner
	// can still be removed independently when callers pass one.
	removeAll := owner == "" || owner == adapter.Owner(Owner)
	change, changed, err := r.staticChange(func(existing []byte) ([]byte, error) {
		if removeAll {
			return removeClaudeMD(existing, "")
		}
		return removeClaudeMD(existing, string(owner))
	})
	if err != nil {
		return adapter.Changeset{}, err
	}
	if changed {
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return adapter.Changeset{Files: changes}, nil
}

func fileChange(file install.PlannedFile) adapter.FileChange {
	return adapter.FileChange{Path: file.Path, Root: file.Root, Mode: file.Mode, Before: file.Before, After: file.After, CreateOnly: file.Before == nil, Remove: file.Remove}
}

func (r *route) staticChange(transform func([]byte) ([]byte, error)) (adapter.FileChange, bool, error) {
	path := filepath.Join(r.dir, "CLAUDE.md")
	before, mode, err := readOptional(path)
	if err != nil {
		return adapter.FileChange{}, false, err
	}
	updated, err := transform(cloneNullable(before))
	if err != nil {
		return adapter.FileChange{}, false, err
	}
	if before == nil {
		if len(updated) == 0 {
			return adapter.FileChange{}, false, nil
		}
		if mode == 0 {
			mode = 0o644
		}
		return adapter.FileChange{Path: path, Root: r.dir, Mode: mode, After: updated, CreateOnly: true}, true, nil
	}
	if bytes.Equal(before, updated) {
		return adapter.FileChange{}, false, nil
	}
	if len(updated) == 0 {
		return adapter.FileChange{Path: path, Root: r.dir, Mode: mode, Before: before, Remove: true}, true, nil
	}
	return adapter.FileChange{Path: path, Root: r.dir, Mode: mode, Before: before, After: updated}, true, nil
}

func readOptional(path string) ([]byte, os.FileMode, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("claude route: read %s: %w", path, err)
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	return data, mode, nil
}

func cloneNullable(data []byte) []byte {
	if data == nil {
		return nil
	}
	return append([]byte(nil), data...)
}

type claudeRegion struct {
	owner      string
	start, end int
}

const claudeSeparator = "<!-- style-managed-separator -->"

func parseClaudeRegions(text string) ([]claudeRegion, error) {
	var regions []claudeRegion
	seen := map[string]bool{}
	var open *claudeRegion
	for offset := 0; offset < len(text); {
		lineEnd := strings.IndexByte(text[offset:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += offset + 1
		}
		line := strings.TrimSuffix(strings.TrimSuffix(text[offset:lineEnd], "\n"), "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "<!-- style:"):
			if open != nil {
				return nil, errors.New("claude route: nested managed CLAUDE.md marker")
			}
			owner, ok := markerOwner(trimmed, "<!-- style:", " -->")
			if !ok {
				return nil, errors.New("claude route: malformed managed CLAUDE.md start marker")
			}
			if seen[owner] {
				return nil, fmt.Errorf("claude route: duplicate managed CLAUDE.md owner %q", owner)
			}
			seen[owner] = true
			open = &claudeRegion{owner: owner, start: offset}
		case strings.HasPrefix(trimmed, "<!-- /style:"):
			if open == nil {
				return nil, errors.New("claude route: orphan managed CLAUDE.md end marker")
			}
			owner, ok := markerOwner(trimmed, "<!-- /style:", " -->")
			if !ok || owner != open.owner {
				return nil, errors.New("claude route: mismatched managed CLAUDE.md end marker")
			}
			open.end = lineEnd
			regions = append(regions, *open)
			open = nil
		default:
			if strings.Contains(trimmed, "<!-- style:") || strings.Contains(trimmed, "<!-- /style:") {
				return nil, errors.New("claude route: malformed managed CLAUDE.md marker")
			}
		}
		offset = lineEnd
	}
	if open != nil {
		return nil, errors.New("claude route: unterminated managed CLAUDE.md block")
	}
	return regions, nil
}

func markerOwner(line, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return "", false
	}
	owner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix))
	if owner == "" || len(owner) > 64 {
		return "", false
	}
	for _, r := range owner {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", false
	}
	return owner, true
}

func staticBlocksText(blocks []adapter.StaticBlock) (string, error) {
	copyBlocks := append([]adapter.StaticBlock(nil), blocks...)
	sort.SliceStable(copyBlocks, func(i, j int) bool { return string(copyBlocks[i].Owner) < string(copyBlocks[j].Owner) })
	seen := map[string]bool{}
	var b strings.Builder
	for i, block := range copyBlocks {
		owner := string(block.Owner)
		if owner == "" {
			return "", errors.New("claude route: static block owner is required")
		}
		if seen[owner] {
			return "", fmt.Errorf("claude route: duplicate static block owner %q", owner)
		}
		seen[owner] = true
		if block.Slot != "" && block.Slot != StaticSlotAppend {
			return "", fmt.Errorf("claude route: unsupported static slot %q", block.Slot)
		}
		if strings.Contains(block.Body, "<!-- style:") || strings.Contains(block.Body, "<!-- /style:") {
			return "", fmt.Errorf("claude route: static block %q contains a reserved managed marker", owner)
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("<!-- style:")
		b.WriteString(owner)
		b.WriteString(" -->\n")
		b.WriteString(block.Body)
		if !strings.HasSuffix(block.Body, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("<!-- /style:")
		b.WriteString(owner)
		b.WriteString(" -->")
	}
	return b.String(), nil
}

func mergeClaudeMD(existing []byte, blocks []adapter.StaticBlock) ([]byte, error) {
	if len(blocks) == 0 {
		return existing, nil
	}
	text := string(existing)
	regions, err := parseClaudeRegions(text)
	if err != nil {
		return nil, err
	}
	blockText, err := staticBlocksText(blocks)
	if err != nil {
		return nil, err
	}
	owners := map[string]bool{}
	for _, block := range blocks {
		owners[string(block.Owner)] = true
	}
	base := removeClaudeRegions(text, regions, owners)
	if len(blockText) == 0 {
		return []byte(base), nil
	}
	if len(base) != 0 {
		switch {
		case strings.HasSuffix(base, "\n"):
		default:
			base += "\n" + claudeSeparator + "\n"
		}
	}
	return []byte(base + blockText + "\n"), nil
}

func removeClaudeMD(existing []byte, owner string) ([]byte, error) {
	text := string(existing)
	regions, err := parseClaudeRegions(text)
	if err != nil {
		return nil, err
	}
	if len(regions) == 0 {
		return existing, nil
	}
	owners := map[string]bool{}
	for _, region := range regions {
		if owner == "" || region.owner == owner {
			owners[region.owner] = true
		}
	}
	if len(owners) == 0 {
		return existing, nil
	}
	return []byte(removeClaudeRegions(text, regions, owners)), nil
}

func removeClaudeRegions(text string, regions []claudeRegion, owners map[string]bool) string {
	var b strings.Builder
	last := 0
	for _, region := range regions {
		if !owners[region.owner] {
			continue
		}
		start, end := region.start, region.end
		if end < len(text) && text[end] == '\n' {
			end++
		}
		b.WriteString(text[last:start])
		last = end
	}
	b.WriteString(text[last:])
	result := b.String()
	separator := "\n" + claudeSeparator + "\n"
	if start := strings.LastIndex(result, separator); start >= 0 && strings.TrimSpace(result[start+len(separator):]) == "" {
		result = result[:start]
	}
	return result
}

func claudeStaticAvailable(data []byte) bool {
	_, err := parseClaudeRegions(string(data))
	return err == nil
}

func claudeStaticOwned(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	regions, err := parseClaudeRegions(string(data))
	return len(regions) > 0, err
}

func claudeSettingsOwned(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var value struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return false, fmt.Errorf("claude route: invalid settings.json: %w", err)
	}
	for _, groups := range value.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if strings.Contains(hook.Command, "--owner="+Owner) {
					return true, nil
				}
				for _, arg := range hook.Args {
					if arg == "--owner="+Owner {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

var _ adapter.Route = (*route)(nil)

// NewUserRouteForInstallation binds a user-level Claude config directory and
// optional detected harness version to the route implementation.
func NewUserRouteForInstallation(dir, harnessVersion, command string, facts *Facts) adapter.Route {
	f := DefaultFacts()
	if facts != nil {
		f = *facts
	}
	if dir == "" {
		dir = defaultClaudeHome()
	}
	return &route{name: UserRouteName, dir: dir, command: command, harnessVersion: harnessVersion, facts: f}
}

func NewUserRoute(dir string, facts *Facts) adapter.Route {
	return NewUserRouteForInstallation(dir, "", "", facts)
}
