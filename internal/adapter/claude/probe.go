package claude

import (
	"context"
	"errors"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

func (r *route) InspectProbe(ctx context.Context, read func(string) ([]byte, error)) (adapter.ProbeInstallation, error) {
	result := adapter.ProbeInstallation{Root: r.dir, ConfigRoot: r.dir, RuntimeCommand: adapter.HookCommand(r.command), HarnessCommand: "claude", Events: []string{}}
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
	return origin.PluginRootSHA256 == ""
}
