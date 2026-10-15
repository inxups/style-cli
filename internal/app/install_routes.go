package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks/install"
)

func bindRoute(route adapter.Route, installation adapter.Installation, home string) adapter.Route {
	if binder, ok := route.(adapter.InstallationBinder); ok {
		route = binder.BindInstallation(installation)
	}
	if binder, ok := route.(adapter.StorageBinder); ok {
		route = binder.BindStorage(home)
	}
	return route
}

func planInstallRoutes(ctx context.Context, home string, adapters []installAdapter, compilation installCompilation, lock lockDocument, failClosed []string) ([]install.PlannedFile, map[string]lockHarness, []string, error) {
	changes := []install.PlannedFile{}
	harnesses := cloneHarnesses(lock.Harnesses)
	routes := []string{}
	for _, target := range adapters {
		prior := lock.Harnesses[string(target.provider)]
		closed := prior.FailClosed || containsProvider(failClosed, target.provider)
		if closed && target.provider != adapter.ProviderID("claude-code") {
			return nil, nil, nil, &UsageError{Message: "--fail-closed is supported only by claude-code"}
		}
		hooks := hooksForProvider(target.provider, compilation.index)
		if closed {
			for i := range hooks {
				hooks[i].Extra = cloneExtra(hooks[i].Extra)
				if hooks[i].Extra == nil {
					hooks[i].Extra = map[string]any{}
				}
				hooks[i].Extra["fail_closed"] = true
			}
		}
		blocks := staticBlocksForProvider(compilation.staticBodies, target.provider)
		var planned adapter.Changeset
		var err error
		if prior.Route != "" && prior.Route != target.route.Name() {
			previous := routeByName(target.definition, prior.Route)
			if previous == nil {
				return nil, nil, nil, &ConflictError{Message: fmt.Sprintf("previous route %q is unavailable", prior.Route)}
			}
			oldInstallation := target.installation
			oldInstallation.ConfigDir = prior.ConfigDir
			previous = bindRoute(previous, oldInstallation, home)
			transition, ok := target.route.(adapter.TransitionPlanner)
			if !ok {
				return nil, nil, nil, &UsageError{Message: fmt.Sprintf("route %s cannot switch from %s", target.route.Name(), prior.Route)}
			}
			planned, err = transition.PlanTransition(ctx, previous, hooks, blocks)
		} else {
			planned, err = target.route.Plan(ctx, hooks, blocks)
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("plan route %s: %w", target.route.Name(), err)
		}
		// Preserve the stale-plan check before applying the complete transaction.
		if _, err := target.route.Status(ctx, target.installation); err != nil {
			return nil, nil, nil, fmt.Errorf("check route %s: %w", target.route.Name(), err)
		}
		changes = append(changes, routePlannedFiles(planned)...)
		rendered := prior.Rendered
		if len(planned.Files) > 0 || rendered == "" {
			rendered = hashChangeset(planned)
		}
		harnesses[string(target.provider)] = lockHarness{Route: target.route.Name(), Version: target.installation.Version,
			ConfigDir: target.installation.ConfigDir, Surface: hashSurface(hooks), Rendered: rendered,
			Trust: trustState(target.route.Trust()), FailClosed: closed}
		routes = append(routes, string(target.provider)+"="+target.route.Name())
	}
	sort.Strings(routes)
	return changes, harnesses, routes, nil
}
