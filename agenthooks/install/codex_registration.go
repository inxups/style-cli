package install

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CodexRegistration observes native marketplace metadata and the plugin cache
// Codex loads hooks from. Like ClaudeRegistration it never edits native
// configuration or executes the CLI; Codex copies the plugin into its cache
// on `codex plugin add`, so generated files alone are never delivery.
type CodexRegistration struct {
	State       string   `json:"state"`
	Marketplace string   `json:"marketplace"`
	Plugin      string   `json:"plugin"`
	Registered  bool     `json:"registered"`
	Enabled     bool     `json:"enabled"`
	Commands    []string `json:"commands"`
}

// codexCacheArtifacts are the plugin files Codex copies into its cache.
var codexCacheArtifacts = []string{"hooks/hooks.json", "bin/style", ".codex-plugin/plugin.json"}

func InspectCodexRegistration(m Manifest, t Target, read func(string) ([]byte, error)) (CodexRegistration, error) {
	id := m.Identity.PluginID
	name, marketplace, _ := strings.Cut(id, "@")
	env := "CODEX_HOME=" + shellQuote(t.ConfigDir) + " "
	result := CodexRegistration{State: "registration-required", Marketplace: t.LocalMarketplaceDir, Plugin: id, Commands: []string{
		env + "codex plugin marketplace add " + shellQuote(t.LocalMarketplaceDir),
		env + "codex plugin add " + shellQuote(id),
		env + "codex plugin remove " + shellQuote(id),
		env + "codex plugin marketplace remove " + shellQuote(marketplace),
	}}
	fail := func(message string) (CodexRegistration, error) {
		result.State = "unknown"
		return result, fmt.Errorf("registration-required: %s", message)
	}
	if name == "" || marketplace == "" || marketplace != t.LocalMarketplaceName || t.ConfigDir == "" || t.LocalMarketplaceDir == "" {
		return fail("local marketplace identity is incomplete")
	}
	data, err := read(filepath.Join(t.ConfigDir, "config.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return fail("cannot read Codex config")
	}
	var config struct {
		Plugins map[string]struct {
			Enabled *bool `toml:"enabled"`
		} `toml:"plugins"`
		Marketplaces map[string]codexProbeMarketplace `toml:"marketplaces"`
	}
	if toml.Unmarshal(data, &config) != nil {
		return fail("cannot parse Codex config")
	}
	registration, known := config.Marketplaces[marketplace]
	if known && (registration.SourceType != "local" || canonicalCodexDir(registration.Source) != canonicalCodexDir(t.LocalMarketplaceDir)) {
		return fail("marketplace is not the expected local directory")
	}
	plugin, recorded := config.Plugins[id]
	if !known || !recorded {
		return result, nil
	}
	result.Registered = true
	if plugin.Enabled == nil || !*plugin.Enabled {
		result.State = "disabled"
		return result, nil
	}
	result.Enabled = true
	rendered, err := Render(m, t)
	if err != nil {
		return result, err
	}
	cache := filepath.Join(t.ConfigDir, "plugins", "cache", marketplace, name, m.Identity.Version)
	for _, path := range codexCacheArtifacts {
		expected, err := fs.ReadFile(rendered, path)
		if err != nil {
			return result, err
		}
		actual, err := read(filepath.Join(cache, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(actual, expected) {
			// Codex runs the cached copy; it must be re-added after STYLE
			// regenerates the plugin.
			result.State = "refresh-required"
			return result, nil
		}
	}
	result.State = "enabled"
	return result, nil
}
