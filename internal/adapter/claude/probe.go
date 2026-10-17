package claude

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

func (r *route) InspectProbe(ctx context.Context, read func(string) ([]byte, error)) (adapter.ProbeInstallation, error) {
	result := adapter.ProbeInstallation{Root: r.dir, ConfigRoot: r.dir, RuntimeCommand: adapter.HookCommand(r.command), HarnessCommand: "claude", Events: []string{}}
	if r.plugin {
		result.Root = r.pluginRoot()
		registration, err := r.Registration(ctx, read)
		if err != nil {
			return result, err
		}
		if registration.State != "enabled" {
			return result, errors.New("registration-required: plugin is not enabled")
		}
	}
	if r.project {
		result.ConfigRoot = r.configUser
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	status, err := install.InspectRuntime(r.manifest(nil), r.target(), read)
	for _, event := range status.Events {
		if !agenthooks.RecognizesNativeEvent(ProviderName, event) && event != "PostModelSwitch" {
			return result, errors.New("unknown runtime event")
		}
	}
	result.Events = status.Events
	return result, err
}
func (r *route) VerifyProbeOrigin(origin adapter.ProbeOrigin) bool {
	if r.plugin {
		return origin.PluginRootSHA256 == fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(r.pluginRoot())))
	}
	return origin.PluginRootSHA256 == ""
}
