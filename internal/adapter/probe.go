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

// HarnessExecutableResolver maps the harness command found on PATH to the
// executable that actually runs as a hook ancestor, for launchers that
// spawn a separate native binary. It reads files only and never executes.
type HarnessExecutableResolver interface {
	ResolveHarnessExecutable(path string) (string, error)
}
