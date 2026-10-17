package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeRegistration observes native metadata. It never grants trust, edits
// native databases, executes the CLI, or treats enabled metadata as delivery.
type ClaudeRegistration struct {
	State       string   `json:"state"`
	Marketplace string   `json:"marketplace"`
	Plugin      string   `json:"plugin"`
	Registered  bool     `json:"registered"`
	Enabled     bool     `json:"enabled"`
	Commands    []string `json:"commands"`
	Residue     []string `json:"native_residue,omitempty"`
}

func InspectClaudeRegistration(t Target, read func(string) ([]byte, error)) (ClaudeRegistration, error) {
	id := "style-runtime@" + t.LocalMarketplaceName
	result := ClaudeRegistration{State: "registration-required", Marketplace: t.LocalMarketplaceDir, Plugin: id, Commands: []string{
		"CLAUDE_CONFIG_DIR=" + shellQuote(t.ConfigDir) + " claude plugin marketplace add " + shellQuote(t.LocalMarketplaceDir),
		"CLAUDE_CONFIG_DIR=" + shellQuote(t.ConfigDir) + " claude plugin install " + shellQuote(id) + " --scope user",
		"CLAUDE_CONFIG_DIR=" + shellQuote(t.ConfigDir) + " claude plugin disable " + shellQuote(id) + " --scope user",
		"CLAUDE_CONFIG_DIR=" + shellQuote(t.ConfigDir) + " claude plugin uninstall " + shellQuote(id) + " --scope user",
	}}
	fail := func(message string) (ClaudeRegistration, error) {
		result.State = "unknown"
		return result, fmt.Errorf("registration-required: %s", message)
	}
	if t.LocalMarketplaceName == "" || t.ConfigDir == "" || t.LocalMarketplaceDir == "" {
		return fail("local marketplace identity is incomplete")
	}
	decode := func(name string, v any) error {
		raw, err := read(filepath.Join(t.ConfigDir, filepath.FromSlash(name)))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if name != "settings.json" {
			result.Residue = append(result.Residue, filepath.Join(t.ConfigDir, filepath.FromSlash(name)))
		}
		if len(raw) > 4<<20 {
			return errors.New("plugin metadata exceeds limit")
		}
		return json.Unmarshal(raw, v)
	}
	var known map[string]struct {
		Source          struct{ Source, Path string }
		InstallLocation string
	}
	if err := decode("plugins/known_marketplaces.json", &known); err != nil {
		return fail("cannot verify known marketplaces")
	}
	var installed struct {
		Version int
		Plugins map[string][]struct{ Scope, InstallPath, Version string }
	}
	if err := decode("plugins/installed_plugins.json", &installed); err != nil {
		return fail("cannot verify installed plugins")
	}
	var settings struct {
		Enabled map[string]bool `json:"enabledPlugins"`
	}
	if err := decode("settings.json", &settings); err != nil {
		return fail("cannot verify plugin enablement")
	}
	marketplace, knownHere := known[t.LocalMarketplaceName]
	if knownHere && (marketplace.Source.Source != "directory" || filepath.Clean(marketplace.Source.Path) != filepath.Clean(t.LocalMarketplaceDir) || filepath.Clean(marketplace.InstallLocation) != filepath.Clean(t.LocalMarketplaceDir)) {
		return fail("marketplace is not the expected local directory")
	}
	records := installed.Plugins[id]
	if len(records) > 1 || len(records) > 0 && (installed.Version != 2 || records[0].Scope != "user" || records[0].Version != "0.1.0" || !knownHere) {
		return fail("plugin has unsupported registration or scope")
	}
	enabled, explicit := settings.Enabled[id]
	result.Registered = len(records) == 1
	if enabled && (!result.Registered || !knownHere) {
		return fail("enabled plugin registration is incomplete")
	}
	if result.Registered && !explicit {
		return fail("registered plugin enablement is unknown")
	}
	if enabled {
		result.State = "enabled"
		result.Enabled = true
	} else if explicit && !enabled {
		result.State = "disabled"
	}
	if len(records) > 0 && records[0].InstallPath != "" {
		result.Residue = append(result.Residue, records[0].InstallPath)
	}
	return result, nil
}
