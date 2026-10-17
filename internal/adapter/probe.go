package adapter

import "context"

// ProbeInstallation contains only read-only route facts. The renderer/owner
// inspector in agenthooks still owns configuration and command interpretation.
type ProbeInstallation struct {
	Root           string
	ConfigRoot     string
	RuntimeCommand string
	HarnessCommand string
	Events         []string
}

type ProbeOrigin struct{ PluginRootSHA256, RouteRootSHA256 string }

type ProbeRoute interface {
	InspectProbe(context.Context, func(string) ([]byte, error)) (ProbeInstallation, error)
	VerifyProbeOrigin(ProbeOrigin) bool
}
