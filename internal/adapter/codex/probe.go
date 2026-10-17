package codex

import (
	"context"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks/install"
)

func (r *route) InspectProbe(ctx context.Context, read func(string) ([]byte, error)) (adapter.ProbeInstallation, error) {
	result := adapter.ProbeInstallation{Root: r.dir, ConfigRoot: r.configRoot(), RuntimeCommand: adapter.HookCommand(r.command), HarnessCommand: "codex", Events: []string{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	status, err := install.InspectRuntime(r.manifest(), r.target(), read)
	result.Events = status.Events
	return result, err
}
func (r *route) VerifyProbeOrigin(origin adapter.ProbeOrigin) bool {
	if r.scope == install.ScopePlugin {
		return origin.PluginRootSHA256 != "" && origin.PluginRootSHA256 == origin.RouteRootSHA256
	}
	return origin.PluginRootSHA256 == ""
}
