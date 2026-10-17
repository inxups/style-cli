package claude

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks/install"
)

func NewPluginRouteForInstallation(dir, version, command string, facts *Facts) adapter.Route {
	r := NewUserRouteForInstallation(dir, version, command, facts).(*route)
	r.name = PluginRouteName
	r.plugin = true
	return r
}
func canonicalPluginHome(home string) string {
	absolute, err := filepath.Abs(home)
	if err != nil {
		return home
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved
	}
	if absolute == filepath.Dir(absolute) {
		return absolute
	}
	return filepath.Join(canonicalPluginHome(filepath.Dir(absolute)), filepath.Base(absolute))
}
func (r *route) marketplaceRoot() string {
	return filepath.Join(canonicalPluginHome(r.storageHome), "adapters", "claude", "marketplace")
}
func (r *route) pluginRoot() string {
	return filepath.Join(r.marketplaceRoot(), "plugins", "style-runtime")
}
func (r *route) marketplaceName() string {
	return fmt.Sprintf("style-%x", sha256.Sum256([]byte(canonicalPluginHome(r.storageHome))))
}
func withinPluginRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func (r *route) registrationRead(path string) ([]byte, error) {
	if err := install.CheckPath(r.dir, path); err != nil {
		return nil, err
	}
	return saferead.ReadFile(path, 4<<20)
}
func (r *route) Registration(ctx context.Context, read func(string) ([]byte, error)) (*adapter.RegistrationReport, error) {
	if !r.plugin {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if read == nil {
		read = r.registrationRead
	}
	v, err := install.InspectClaudeRegistration(r.target(), read)
	return &adapter.RegistrationReport{Route: r.name, State: v.State, Marketplace: v.Marketplace, Plugin: v.Plugin, Registered: v.Registered, Enabled: v.Enabled, Commands: v.Commands, Residue: v.Residue}, err
}
func (r *route) pluginStatus(ctx context.Context, installation adapter.Installation) (adapter.RouteStatus, error) {
	available, detail, err := r.Available(ctx, installation)
	result := adapter.RouteStatus{Available: available, Detail: detail}
	if err != nil || !available {
		return result, err
	}
	registration, err := r.Registration(ctx, nil)
	if err != nil {
		result.Detail = err.Error()
		return result, nil
	}
	result.Detail = registration.State + ": " + detail
	_, err = install.InspectRuntime(r.manifest(nil), r.target(), func(path string) ([]byte, error) {
		if e := install.CheckPath(r.marketplaceRoot(), path); e != nil {
			return nil, e
		}
		return saferead.ReadFile(path, 4<<20)
	})
	if err != nil {
		result.Detail += "; generated artifacts not verified: " + err.Error()
		return result, nil
	}
	result.Installed = true
	return result, nil
}
func (r *route) planPlugin(ctx context.Context, hooks []adapter.HookSpec, blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	return r.planPluginAfterUserRemoval(ctx, hooks, blocks, false)
}
func (r *route) planPluginAfterUserRemoval(ctx context.Context, hooks []adapter.HookSpec, blocks []adapter.StaticBlock, userRemoval bool) (adapter.Changeset, error) {
	if !userRemoval {
		owned, err := claudeSettingsOwned(filepath.Join(r.dir, "settings.json"))
		if err != nil {
			return adapter.Changeset{}, err
		}
		if owned {
			return adapter.Changeset{}, fmt.Errorf("claude plugin: user STYLE hooks are also present; switch explicitly from the owning installation")
		}
	}
	if r.storageHome == "" {
		return adapter.Changeset{}, fmt.Errorf("claude plugin requires explicit storage binding")
	}
	if _, err := r.Registration(ctx, nil); err != nil {
		return adapter.Changeset{}, err
	}
	files, err := install.PlanFiles(r.manifest(hooks), r.target())
	if err != nil {
		return adapter.Changeset{}, err
	}
	changes, err := r.rulesChanges(blocks)
	if err != nil {
		return changes, err
	}
	for _, file := range files {
		changes.Files = append(changes.Files, fileChange(file))
	}
	sort.Slice(changes.Files, func(i, j int) bool { return changes.Files[i].Path < changes.Files[j].Path })
	return changes, nil
}
func (r *route) removePluginHooks(ctx context.Context) ([]install.PlannedFile, error) {
	registration, err := r.Registration(ctx, nil)
	if err != nil {
		return nil, err
	}
	if registration.Enabled {
		return nil, fmt.Errorf("registration-required: disable or uninstall %s with Claude CLI before removing or switching its source; %s", registration.Plugin, registration.Commands[2])
	}
	return install.PlanUninstallFiles(r.manifest(nil), r.target())
}
func (r *route) removePlugin(ctx context.Context, owner adapter.Owner) (adapter.Changeset, error) {
	files, err := r.removePluginHooks(ctx)
	if err != nil {
		return adapter.Changeset{}, err
	}
	// Reuse the existing static ownership and migration path without touching
	// user settings hook definitions or native registration databases.
	user := *r
	user.plugin = false
	static, err := user.Remove(ctx, owner)
	if err != nil {
		return adapter.Changeset{}, err
	}
	result := adapter.Changeset{Static: static.Static, Migrations: static.Migrations}
	for _, file := range static.Files {
		if filepath.Base(file.Path) == "settings.json" || strings.Contains(file.Path, string(filepath.Separator)+".agenthooks"+string(filepath.Separator)) {
			continue
		}
		result.Files = append(result.Files, file)
	}
	for _, file := range files {
		result.Files = append(result.Files, fileChange(file))
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}
func (r *route) PlanTransition(ctx context.Context, previous adapter.Route, hooks []adapter.HookSpec, blocks []adapter.StaticBlock) (adapter.Changeset, error) {
	old, ok := previous.(*route)
	if !ok || r.project || old.project || r.dir != old.dir {
		return adapter.Changeset{}, fmt.Errorf("claude route: incompatible transition")
	}
	if r.plugin {
		status, err := r.Registration(ctx, nil)
		if err != nil {
			return adapter.Changeset{}, err
		}
		if status.Enabled {
			return adapter.Changeset{}, fmt.Errorf("registration-required: disable the target plugin before switching from user hooks")
		}
	}
	var removed []install.PlannedFile
	var err error
	if old.plugin {
		removed, err = old.removePluginHooks(ctx)
	} else {
		removed, err = install.PlanUninstallFiles(old.manifest(nil), old.target())
	}
	if err != nil {
		return adapter.Changeset{}, err
	}
	var next adapter.Changeset
	if r.plugin {
		next, err = r.planPluginAfterUserRemoval(ctx, hooks, blocks, true)
	} else {
		next, err = r.Plan(ctx, hooks, blocks)
	}
	if err != nil {
		return next, err
	}
	for _, file := range removed {
		next.Files = append(next.Files, fileChange(file))
	}
	sort.Slice(next.Files, func(i, j int) bool { return next.Files[i].Path < next.Files[j].Path })
	return next, nil
}

var _ adapter.RegistrationInspector = (*route)(nil)
var _ adapter.TransitionPlanner = (*route)(nil)
