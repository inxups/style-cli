// Package codex contains Codex-specific data resolution used by the adapter.
package codex

import (
	"embed"
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
	ProjectTrusted bool
	Launch         agenthooks.CodexLaunchSettings
	ReadFile       func(string) ([]byte, error)
}

// EffortResolution records a canonical value and its safe configuration source.
type EffortResolution struct {
	effortmap.Value
	Source string
}

// ResolveEffort follows the documented order: launch -c, selected profile,
// trusted project config, then user config. When trust is unknown, the project
// layer is skipped.
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
