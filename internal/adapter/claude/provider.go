package claude

import (
	"os"
	"strings"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/speakeasy-api/agenthooks"
)

type ProviderResolution struct{ ID, Source string }

// ResolveProvider reads only service selectors and endpoint presence. It never
// reads credentials or identifies a gateway's upstream from its URL.
func ResolveProvider(lookup func(string) (string, bool)) ProviderResolution {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	unknown := func(reason string) ProviderResolution { return ProviderResolution{Source: "claude-provider-" + reason} }
	for _, name := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_BEDROCK_BASE_URL", "ANTHROPIC_VERTEX_BASE_URL", "ANTHROPIC_FOUNDRY_BASE_URL", "ANTHROPIC_AWS_BASE_URL", "CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST", "CLAUDE_CODE_HOST_MANAGED"} {
		if value, _ := lookup(name); strings.TrimSpace(value) != "" {
			return unknown("managed-or-endpoint")
		}
	}
	selected := ""
	for _, flag := range []struct{ key, id string }{{"CLAUDE_CODE_USE_BEDROCK", "bedrock"}, {"CLAUDE_CODE_USE_VERTEX", "vertex"}, {"CLAUDE_CODE_USE_FOUNDRY", "foundry"}, {"CLAUDE_CODE_USE_MANTLE", ""}, {"ANTHROPIC_AWS_USE_MANTLE", ""}, {"CLAUDE_CODE_USE_ANTHROPIC_AWS", ""}} {
		raw, _ := lookup(flag.key)
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "", "0", "false":
			continue
		case "1", "true":
			if flag.id == "" {
				return unknown("unsupported-route")
			}
			if selected != "" {
				return unknown("conflicting-selectors")
			}
			selected = flag.id
		default:
			return unknown("invalid-selector")
		}
	}
	if selected == "" {
		return ProviderResolution{"anthropic", "claude-default"}
	}
	return ProviderResolution{selected, "claude-environment"}
}

func (e *Enricher) ConfigureProvider(enabled bool) {
	if e != nil {
		e.providerEnabled = enabled
	}
}

func (e *Enricher) updateProvider(update *state.EventUpdate, _ *agenthooks.Event, _ state.Session) {
	if e == nil || !e.providerEnabled {
		return
	}
	update.ProviderResolved = true
	if e.ProviderOffline {
		update.ProviderSource = "offline-unavailable"
		if e.OfflineProvider != nil && when.ValidProviderID(*e.OfflineProvider) {
			update.ModelProvider = *e.OfflineProvider
			update.ProviderSource = "provided"
		}
		return
	}
	resolution := ResolveProvider(e.ProviderEnvironment)
	update.ModelProvider = resolution.ID
	update.ProviderSource = resolution.Source
}
