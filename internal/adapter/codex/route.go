package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks/install"
)

const staticBegin = "# >>> style (managed; edit with `style`) >>>"
const staticEnd = "# <<< style <<<"

type route struct {
	name           string
	scope          install.Scope
	dir            string
	configDir      string
	command        string
	facts          Facts
	defaultDir     bool
	storageHome    string
	harnessVersion string
}

func (r *route) Name() string { return r.name }

func (r *route) BindStorage(home string) adapter.Route {
	bound := *r
	bound.storageHome = home
	if bound.defaultDir && bound.scope == install.ScopePlugin {
		bound.dir = filepath.Join(home, "adapters", "codex", "plugin")
	}
	return &bound
}

func (r *route) BindInstallation(installation adapter.Installation) adapter.Route {
	bound := *r
	bound.harnessVersion = installation.Version
	if installation.ConfigDir != "" {
		bound.configDir = installation.ConfigDir
		if bound.scope == install.ScopeUser {
			bound.dir = installation.ConfigDir
		}
	}
	return &bound
}

func (r *route) Available(_ context.Context, installation adapter.Installation) (bool, string, error) {
	if installation.Provider != "" && installation.Provider != ProviderName {
		return false, "wrong provider", nil
	}
	if r.scope == install.ScopePlugin && r.configDir == "" {
		return false, "CODEX_HOME is required for plugin trust state", nil
	}
	if r.dir == "" {
		return false, "route directory is required", nil
	}
	return true, "available", nil
}

func (r *route) Trust() adapter.TrustInfo {
	if r.scope == install.ScopePlugin {
		return adapter.TrustInfo{Kind: adapter.TrustUserConfirmation, StableKey: true, Summary: "trust the style-runtime plugin once"}
	}
	return adapter.TrustInfo{Kind: adapter.TrustPreseed, StableKey: true, Summary: "Codex hooks.state is pre-seeded"}
}

func (r *route) Status(ctx context.Context, installation adapter.Installation) (adapter.RouteStatus, error) {
	available, detail, err := r.Available(ctx, installation)
	if err != nil || !available {
		return adapter.RouteStatus{Available: available, Detail: detail}, err
	}
	native, err := r.installedNative()
	if err != nil {
		return adapter.RouteStatus{Available: true, Detail: err.Error()}, nil
	}
	changes, err := install.Diff(r.manifest(native), r.target())
	if err != nil {
		return adapter.RouteStatus{Available: true, Detail: err.Error()}, nil
	}
	installed := true
	for _, change := range changes {
		if change.State != install.StateUnchanged {
			installed = false
			break
		}
	}
	// A plugin route needs an explicit user confirmation that cannot be
	// inferred from the presence of generated files. User-level preseed trust
	// is established by the managed config itself once the route is installed.
	trusted := installed && r.scope == install.ScopeUser
	return adapter.RouteStatus{Available: true, Installed: installed, Trusted: trusted, Detail: detail}, nil
}

func (r *route) target() install.Target {
	return install.Target{Provider: ProviderName, Scope: r.scope, Dir: r.dir, ConfigDir: r.configDir}
}

func (r *route) manifest(requests ...[]adapter.HookSpec) install.Manifest {
	argv := []string{adapter.HookCommand(r.command)}
	if r.storageHome != "" {
		argv = append(argv, "--style-home="+r.storageHome, "--harness-home="+r.configRoot(), "--harness-version="+r.harnessVersion)
	}
	return HookManifest(argv, requests...)
}

func (r *route) Plan(_ context.Context, hooks []adapter.HookSpec, blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	manifest := r.manifest(hooks)
	planned, err := install.PlanFiles(manifest, r.target())
	if err != nil {
		return adapter.Changeset{}, err
	}
	return r.planStatic(planned, blocks)
}

func (r *route) InspectNative(_ context.Context, hooks []adapter.HookSpec) ([]adapter.NativeStatus, error) {
	statuses, err := install.InspectNative(r.manifest(hooks), r.target())
	result := make([]adapter.NativeStatus, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, adapter.NativeStatus{Tag: status.Tag, State: status.State, Trust: status.Trust})
	}
	return result, err
}

func (r *route) planStatic(planned []install.PlannedFile, blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	if len(blocks) > 0 && !r.facts.StaticAppendAvailable {
		return adapter.Changeset{}, errors.New("codex route: developer_instructions is already user-owned")
	}
	changes := make([]adapter.FileChange, 0, len(planned)+1)
	for _, file := range planned {
		if filepath.Clean(file.Path) == filepath.Clean(r.configPath()) {
			continue
		}
		changes = append(changes, fileChange(file))
	}
	config, changed, err := r.configChange(planned, func(after []byte) ([]byte, error) {
		return mergeStaticTOML(after, blocks)
	})
	if err != nil {
		return adapter.Changeset{}, err
	}
	if changed {
		changes = append(changes, config)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return adapter.Changeset{Files: changes}, nil
}

func (r *route) PlanTransition(ctx context.Context, previous adapter.Route, hooks []adapter.HookSpec, blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	if _, ok := previous.(*route); !ok {
		return adapter.Changeset{}, errors.New("codex route: incompatible previous route")
	}
	removed, err := previous.Remove(ctx, adapter.Owner(Owner))
	if err != nil {
		return adapter.Changeset{}, err
	}
	files := make([]install.PlannedFile, 0, len(removed.Files))
	for _, file := range removed.Files {
		files = append(files, install.PlannedFile{Path: file.Path, Root: file.Root, Before: file.Before, After: file.After, Mode: file.Mode, Remove: file.Remove})
	}
	planned, err := install.PlanFilesAfter(r.manifest(hooks), r.target(), files)
	if err != nil {
		return adapter.Changeset{}, err
	}
	return r.planStatic(planned, blocks)
}

func (r *route) Apply(ctx context.Context, changes adapter.Changeset) error {
	files := make([]install.PlannedFile, 0, len(changes.Files))
	for _, change := range changes.Files {
		root, _, err := r.splitDestination(change.Path)
		if err != nil {
			return err
		}
		files = append(files, install.PlannedFile{Root: root, Path: change.Path, Before: change.Before, After: change.After, Mode: change.Mode, Remove: change.Remove})
	}
	return install.ApplyFiles(ctx, files)
}

func (r *route) Remove(_ context.Context, owner adapter.Owner) (adapter.Changeset, error) {
	manifest := r.manifest()
	if owner != "" {
		manifest.Owner = string(owner)
	}
	planned, err := install.PlanUninstallFiles(manifest, r.target())
	if err != nil {
		return adapter.Changeset{}, err
	}
	changes := make([]adapter.FileChange, 0, len(planned)+1)
	for _, file := range planned {
		if filepath.Base(file.Path) == "config.toml" {
			continue
		}
		changes = append(changes, fileChange(file))
	}
	if owner == "" || owner == adapter.Owner(Owner) {
		config, changed, err := r.configChange(planned, func(after []byte) ([]byte, error) {
			return removeStaticTOML(after, string(owner))
		})
		if err != nil {
			return adapter.Changeset{}, err
		}
		if changed {
			changes = append(changes, config)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return adapter.Changeset{Files: changes}, nil
}

func fileChange(file install.PlannedFile) adapter.FileChange {
	return adapter.FileChange{
		Path:       file.Path,
		Root:       file.Root,
		Mode:       file.Mode,
		Before:     file.Before,
		After:      file.After,
		CreateOnly: file.Before == nil,
		Remove:     file.Remove,
	}
}

func (r *route) configChange(planned []install.PlannedFile, transform func([]byte) ([]byte, error)) (adapter.FileChange, bool, error) {
	path := r.configPath()
	var source *install.PlannedFile
	for i := range planned {
		if filepath.Clean(planned[i].Path) == filepath.Clean(path) {
			source = &planned[i]
			break
		}
	}

	var before, after []byte
	mode := os.FileMode(0o644)
	if source != nil {
		before = cloneNullable(source.Before)
		after = cloneNullable(source.After)
		mode = source.Mode
	} else {
		data, err := os.ReadFile(path)
		if err == nil {
			before = data
			after = append([]byte(nil), data...)
			if info, statErr := os.Stat(path); statErr == nil {
				mode = info.Mode().Perm()
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return adapter.FileChange{}, false, fmt.Errorf("codex route: read config.toml: %w", err)
		}
	}
	updated, err := transform(after)
	if err != nil {
		return adapter.FileChange{}, false, err
	}
	remove := source != nil && source.Remove || before != nil && len(before) > 0 && len(updated) == 0
	if !remove && bytes.Equal(before, updated) {
		return adapter.FileChange{}, false, nil
	}
	return adapter.FileChange{
		Path:       path,
		Root:       r.configRoot(),
		Mode:       mode,
		Before:     before,
		After:      updated,
		CreateOnly: before == nil,
		Remove:     remove,
	}, true, nil
}

func cloneNullable(data []byte) []byte {
	if data == nil {
		return nil
	}
	return append([]byte{}, data...)
}

func (r *route) configPath() string {
	return filepath.Join(r.configRoot(), "config.toml")
}

func (r *route) configRoot() string {
	root := r.dir
	if r.scope == install.ScopePlugin {
		root = r.configDir
	}
	return root
}

func NewPluginRouteForCommand(dir, configDir, command string, facts *Facts) adapter.Route {
	f := DefaultFacts()
	if facts != nil {
		f = *facts
	}
	defaultDir := dir == ""
	if defaultDir {
		dir = defaultPluginDir()
	}
	if configDir == "" {
		configDir = defaultCodexHome()
	}
	return &route{name: PluginRouteName, scope: install.ScopePlugin, dir: dir, configDir: configDir, command: command, facts: f, defaultDir: defaultDir}
}

func NewUserRouteForCommand(dir, command string, facts *Facts) adapter.Route {
	f := DefaultFacts()
	if facts != nil {
		f = *facts
	}
	if dir == "" {
		dir = defaultCodexHome()
	}
	return &route{name: UserRouteName, scope: install.ScopeUser, dir: dir, command: command, facts: f}
}

func defaultCodexHome() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func defaultPluginDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".style", "adapters", "codex", "plugin")
	}
	return filepath.Join(home, ".style", "adapters", "codex", "plugin")
}

func (r *route) splitDestination(destination string) (root, relative string, err error) {
	candidates := []string{r.configDir, r.dir}
	sort.SliceStable(candidates, func(i, j int) bool { return len(candidates[i]) > len(candidates[j]) })
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		rel, relErr := filepath.Rel(candidate, destination)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return candidate, rel, nil
		}
	}
	return "", "", fmt.Errorf("codex route: destination %q is outside route roots", destination)
}

func staticBlocksText(blocks []adapter.StaticBlock) string {
	copyBlocks := append([]adapter.StaticBlock(nil), blocks...)
	sort.SliceStable(copyBlocks, func(i, j int) bool { return string(copyBlocks[i].Owner) < string(copyBlocks[j].Owner) })
	var b strings.Builder
	for i, block := range copyBlocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("<!-- style:")
		b.WriteString(string(block.Owner))
		b.WriteString(" -->\n")
		b.WriteString(block.Body)
		b.WriteString("\n<!-- /style:")
		b.WriteString(string(block.Owner))
		b.WriteString(" -->")
	}
	return b.String()
}

func staticSlotAvailable(data []byte) bool {
	text := string(data)
	if strings.Contains(text, staticBegin) {
		_, err := parseStaticTOML(data)
		_, _, managed, regionErr := staticRegion(text)
		if err != nil || regionErr != nil || !managed || !hasManagedStaticAssignment(text) {
			return false
		}
		return true
	}
	parsed, err := parseStaticTOML(data)
	if err != nil {
		return false
	}
	_, exists := parsed[StaticSlotAppend]
	return !exists
}

func mergeStaticTOML(existing []byte, blocks []adapter.StaticBlock) ([]byte, error) {
	if len(blocks) == 0 {
		return removeStaticTOML(existing, string(Owner))
	}
	for _, block := range blocks {
		if block.Slot != "" && block.Slot != StaticSlotAppend {
			return nil, fmt.Errorf("codex route: unsupported static slot %q", block.Slot)
		}
	}
	text := string(existing)
	parsed, err := parseStaticTOML(existing)
	if err != nil {
		return nil, fmt.Errorf("codex route: invalid config.toml: %w", err)
	}
	value := staticBlocksText(blocks)
	encoded, err := toml.Marshal(map[string]string{StaticSlotAppend: value})
	if err != nil {
		return nil, fmt.Errorf("codex route: encode developer_instructions: %w", err)
	}
	region := staticBegin + "\n" + string(encoded) + staticEnd + "\n"
	start, end, managed, err := staticRegion(text)
	if err != nil {
		return nil, err
	}
	if managed {
		if !hasManagedStaticAssignment(text) {
			return nil, errors.New("codex route: managed developer_instructions region is malformed")
		}
		text = text[:start] + text[end:]
		remaining, parseErr := parseStaticTOML([]byte(text))
		if parseErr != nil {
			return nil, fmt.Errorf("codex route: config.toml outside managed region is invalid: %w", parseErr)
		}
		if _, exists := remaining[StaticSlotAppend]; exists {
			return nil, errors.New("codex route: developer_instructions is already user-owned")
		}
		return []byte(region + text), nil
	}
	if _, exists := parsed[StaticSlotAppend]; exists {
		return nil, errors.New("codex route: developer_instructions is already user-owned")
	}
	return []byte(region + text), nil
}

func removeStaticTOML(existing []byte, owner string) ([]byte, error) {
	if owner != "" && owner != string(Owner) {
		return existing, nil
	}
	text := string(existing)
	start, end, managed, err := staticRegion(text)
	if err != nil {
		return nil, err
	}
	if !managed {
		return existing, nil
	}
	parsed, err := parseStaticTOML(existing)
	value, exists := parsed[StaticSlotAppend]
	_, isString := value.(string)
	if err != nil || !hasManagedStaticAssignment(text) || !exists || !isString {
		return nil, errors.New("codex route: managed developer_instructions region is malformed")
	}
	result := []byte(text[:start] + text[end:])
	if _, err := parseStaticTOML(result); err != nil {
		return nil, fmt.Errorf("codex route: removing static block produced invalid config.toml: %w", err)
	}
	return result, nil
}

func staticRegion(text string) (start, end int, found bool, err error) {
	beginCount, endCount := strings.Count(text, staticBegin), strings.Count(text, staticEnd)
	if beginCount == 0 && endCount == 0 {
		return -1, -1, false, nil
	}
	if beginCount != 1 || endCount != 1 {
		return -1, -1, false, errors.New("codex route: duplicate or orphan developer_instructions managed marker")
	}
	start, markerEnd := strings.Index(text, staticBegin), strings.Index(text, staticEnd)
	if markerEnd < start {
		return -1, -1, false, errors.New("codex route: incomplete developer_instructions managed region")
	}
	end = markerEnd + len(staticEnd)
	if strings.HasPrefix(text[end:], "\r\n") {
		end += 2
	} else if strings.HasPrefix(text[end:], "\n") {
		end++
	}
	return start, end, true, nil
}

func hasManagedStaticAssignment(text string) bool {
	start, end, found, err := staticRegion(text)
	if err != nil || !found || end < 0 {
		return false
	}
	region := text[start:end]
	for _, line := range strings.Split(region, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, StaticSlotAppend) && strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, StaticSlotAppend)), "=") {
			return true
		}
	}
	return false
}

func parseStaticTOML(data []byte) (map[string]any, error) {
	var parsed map[string]any
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

var _ adapter.Route = (*route)(nil)
