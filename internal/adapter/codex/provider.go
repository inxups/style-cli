package codex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
)

// ProviderInput describes the recoverable launch and provider configuration.
// Project configuration deliberately cannot select a model provider.
type ProviderInput struct {
	EffortInput
	LaunchKnown bool
}

type ProviderResolution struct{ ID, Source string }

// ResolveProvider returns a configured local service request, never an inferred
// vendor or the gateway's effective upstream. Errors remain fixed reason codes.
func ResolveProvider(input ProviderInput) ProviderResolution {
	unavailable := func(reason string) ProviderResolution { return ProviderResolution{Source: "codex-provider-" + reason} }
	if !input.LaunchKnown {
		return unavailable("ancestry-unavailable")
	}
	if input.Launch.ModelProviderSet || input.Launch.ModelProvider != "" {
		if !when.ValidProviderID(input.Launch.ModelProvider) {
			return unavailable("invalid-launch")
		}
		return ProviderResolution{input.Launch.ModelProvider, "codex-launch"}
	}
	if input.Launch.IgnoreUserConfig {
		return unavailable("unreplayable")
	}
	if !fileProfiles(input.HarnessVersion) {
		return unavailable("version-unavailable")
	}
	home := input.Home
	if home == "" {
		home = os.Getenv("CODEX_HOME")
	}
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return unavailable("config-unavailable")
		}
		home = filepath.Join(user, ".codex")
	}
	reader := input.ReadFile
	if reader == nil {
		reader = func(path string) ([]byte, error) { return saferead.ReadFile(path, 4<<20) }
	}
	read := func(path string) (map[string]any, error) {
		data, err := reader(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if len(data) > 4<<20 {
			return nil, errors.New("provider config limit")
		}
		return readConfig(func(string) ([]byte, error) { return data, nil }, path)
	}
	user, err := read(filepath.Join(home, "config.toml"))
	if err != nil {
		return unavailable("config-unavailable")
	}
	if _, legacy := user["profile"]; legacy {
		return unavailable("config-unavailable")
	}
	profile := input.Launch.Profile
	var selected map[string]any
	if profile != "" {
		if len(profile) > 64 || profile == "." || profile == ".." || strings.ContainsAny(profile, `/\`) {
			return unavailable("config-unavailable")
		}
		if nested(user, "profiles", profile) != nil {
			return unavailable("config-unavailable")
		}
		selected, err = read(filepath.Join(home, profile+".config.toml"))
		if err != nil {
			return unavailable("config-unavailable")
		}
	}
	for _, layer := range []struct {
		config map[string]any
		source string
	}{{selected, "codex-profile"}, {user, "codex-user"}} {
		if raw, exists := layer.config["model_provider"]; exists {
			id, ok := raw.(string)
			if !ok || !when.ValidProviderID(id) {
				return unavailable("config-unavailable")
			}
			return ProviderResolution{id, layer.source}
		}
	}
	return ProviderResolution{"openai", "codex-default"}
}

func (e *Enricher) ConfigureProvider(enabled bool) {
	if e != nil {
		e.providerEnabled = enabled
	}
}

func (e *Enricher) updateProvider(update *state.EventUpdate, event *agenthooks.Event, previous state.Session) {
	if e == nil || !e.providerEnabled {
		return
	}
	if e.ProviderOffline {
		update.ProviderResolved = true
		update.ProviderSource = "offline-unavailable"
		if e.OfflineProvider != nil && when.ValidProviderID(*e.OfflineProvider) {
			update.ModelProvider = *e.OfflineProvider
			update.ProviderSource = "provided"
		}
		return
	}
	if previous.ProviderResolved && update.Source != "startup" {
		return
	}
	input := ProviderInput{EffortInput: EffortInput{Home: e.Home, CWD: event.Session.CWD, HarnessVersion: update.HarnessVersion, ReadFile: e.ReadFile}}
	if e.LaunchSettings != nil {
		input.Launch, input.LaunchKnown = e.LaunchSettings(event.Session.CWD)
	} else {
		input.Launch, input.LaunchKnown = agenthooks.CurrentCodexLaunchSettings(event.Session.CWD)
	}
	resolution := ResolveProvider(input)
	update.ModelProvider = resolution.ID
	update.ProviderSource = resolution.Source
	update.ProviderResolved = true
}
