// Package codex contains Codex-specific data resolution used by the adapter.
package codex

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter/effortmap"
	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks"
)

//go:embed effort.yaml
var effortFiles embed.FS

// EffortTable loads the provider's checked-in, validated effort mapping.
func EffortTable() (effortmap.Table, error) {
	file, err := effortFiles.Open("effort.yaml")
	if err != nil {
		return effortmap.Table{}, err
	}
	defer file.Close()
	return effortmap.Parse(file)
}

// EffortInput holds the allowlisted launch facts and trust decision needed to
// resolve Codex's configured request without exposing arbitrary -c values.
type EffortInput struct {
	Home           string
	CWD            string
	HarnessVersion string
	ProjectTrusted bool
	Launch         agenthooks.CodexLaunchSettings
	ReadFile       func(string) ([]byte, error)
}

// EffortResolution records a canonical value and its safe configuration source.
type EffortResolution struct {
	effortmap.Value
	Source string
}

// ResolveEffort resolves configuration requests, not the model's own default.
// Codex 0.159.3 uses named profile files and project layers override profiles.
// Earlier versions retain the legacy table resolver. Unknown project trust
// always excludes project configuration.
func ResolveEffort(input EffortInput) (EffortResolution, error) {
	table, err := EffortTable()
	if err != nil {
		return EffortResolution{}, err
	}
	if raw := strings.TrimSpace(input.Launch.ModelReasoningEffort); raw != "" {
		return mapped(table, raw, "launch -c"), nil
	}
	reader := input.ReadFile
	if reader == nil {
		reader = os.ReadFile
	}
	home := input.Home
	if home == "" {
		home = os.Getenv("CODEX_HOME")
	}
	if home == "" {
		userHome, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return EffortResolution{}, fmt.Errorf("codex effort: locate user home: %w", homeErr)
		}
		home = filepath.Join(userHome, ".codex")
	}
	var userConfig map[string]any
	if !input.Launch.IgnoreUserConfig {
		userConfig, _ = readConfig(reader, filepath.Join(home, "config.toml"))
	}
	if fileProfiles(input.HarnessVersion) {
		return resolveFileProfile(input, reader, home, userConfig, table)
	}
	if !input.Launch.IgnoreUserConfig {
		profile := input.Launch.Profile
		if profile == "" {
			profile = stringValue(userConfig["profile"])
		}
		if profile != "" {
			if raw := stringValue(nested(userConfig, "profiles", profile, "model_reasoning_effort")); raw != "" {
				return mapped(table, raw, "profile"), nil
			}
		}
	}
	if input.ProjectTrusted && input.CWD != "" {
		projectConfig, _ := readConfig(reader, filepath.Join(input.CWD, ".codex", "config.toml"))
		if raw := stringValue(projectConfig["model_reasoning_effort"]); raw != "" {
			return mapped(table, raw, "project config"), nil
		}
	}
	if raw := stringValue(userConfig["model_reasoning_effort"]); raw != "" {
		return mapped(table, raw, "user config"), nil
	}
	return EffortResolution{}, nil
}

// fileProfiles pins the observed format boundary to the tested release. Do not
// infer a new format for an unknown/older installation from this process's CLI.
func fileProfiles(version string) bool {
	var major, minor, patch int
	if n, _ := fmt.Sscanf(strings.TrimPrefix(version, "v"), "%d.%d.%d", &major, &minor, &patch); n != 3 {
		return false
	}
	return major > 0 || minor > 159 || minor == 159 && patch >= 3
}

func resolveFileProfile(input EffortInput, reader func(string) ([]byte, error), home string, userConfig map[string]any, table effortmap.Table) (EffortResolution, error) {
	profile := input.Launch.Profile
	if stringValue(userConfig["profile"]) != "" {
		return EffortResolution{}, fmt.Errorf("codex effort: legacy profile selector is unsupported by this harness version")
	}
	var profileConfig map[string]any
	if profile != "" {
		if profile == "." || profile == ".." || strings.ContainsAny(profile, `/\\`) {
			return EffortResolution{}, fmt.Errorf("codex effort: profile name must not contain a path")
		}
		if _, exists := nested(userConfig, "profiles", profile).(map[string]any); exists {
			return EffortResolution{}, fmt.Errorf("codex effort: selected legacy profile table is unsupported by this harness version")
		}
		var err error
		profileConfig, err = readConfig(reader, filepath.Join(home, profile+".config.toml"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return EffortResolution{}, fmt.Errorf("codex effort: cannot read selected profile: %w", err)
		}
	}
	if input.ProjectTrusted && input.CWD != "" {
		projectConfig, _ := readConfig(reader, filepath.Join(input.CWD, ".codex", "config.toml"))
		if raw := stringValue(projectConfig["model_reasoning_effort"]); raw != "" {
			return mapped(table, raw, "project config"), nil
		}
	}
	if raw := stringValue(profileConfig["model_reasoning_effort"]); raw != "" {
		return mapped(table, raw, "profile file"), nil
	}
	if raw := stringValue(userConfig["model_reasoning_effort"]); raw != "" {
		return mapped(table, raw, "user config"), nil
	}
	return EffortResolution{}, nil
}

// StateUpdate resolves a session's configured effort once and marks even an
// unavailable result as resolved, so every tool hook does not reread config.
func StateUpdate(input EffortInput, previous state.Session) state.EventUpdate {
	update := state.EventUpdate{}
	if previous.EffortResolved {
		return update
	}
	resolution, err := ResolveEffort(input)
	if err != nil {
		return update
	}
	update.ThinkingEffort = resolution.Effort
	update.EffortRaw = resolution.EffortRaw
	update.EffortApprox = cloneBool(resolution.EffortApprox)
	update.EffortResolved = true
	return update
}

func mapped(table effortmap.Table, raw, source string) EffortResolution {
	return EffortResolution{Value: table.Resolve(raw), Source: source}
}

func readConfig(read func(string) ([]byte, error), path string) (map[string]any, error) {
	data, err := read(path)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func nested(root map[string]any, keys ...string) any {
	var current any = root
	for _, key := range keys {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapping[key]
	}
	return current
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
