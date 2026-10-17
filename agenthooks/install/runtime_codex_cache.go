package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type codexProbeMarketplace struct {
	SourceType string `toml:"source_type"`
	Source     string `toml:"source"`
}

// Inspect only the fixed registered local source and its execution artifacts.
// Native management remains responsible for copying or refreshing the cache.
func inspectCodexProbeCache(m Manifest, t Target, marketplaces map[string]codexProbeMarketplace, sourceFiles map[string][]byte, read func(string) ([]byte, error)) (string, error) {
	name, marketplace, ok := strings.Cut(m.Identity.PluginID, "@")
	registration, registered := marketplaces[marketplace]
	if !registered {
		return "", nil
	}
	invalid := errors.New("runtime plugin cache or local registration differs; refresh with native plugin add")
	if !ok || name != m.Identity.Name || !probeCacheComponent(name) || !probeCacheComponent(marketplace) || !probeCacheComponent(m.Identity.Version) || registration.SourceType != "local" || !filepath.IsAbs(registration.Source) {
		return "", invalid
	}
	data, err := read(filepath.Join(registration.Source, ".agents", "plugins", "marketplace.json"))
	if err != nil {
		return "", invalid
	}
	var catalog struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Source string `json:"source"`
				Path   string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &catalog) != nil || catalog.Name != marketplace {
		return "", invalid
	}
	matches := 0
	for _, plugin := range catalog.Plugins {
		if plugin.Name != name {
			continue
		}
		matches++
		path := plugin.Source.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(registration.Source, path)
		}
		if plugin.Source.Source != "local" || plugin.Source.Path == "" || canonicalCodexDir(path) != canonicalCodexDir(t.Dir) {
			return "", invalid
		}
	}
	if matches != 1 {
		return "", invalid
	}
	root := filepath.Join(t.ConfigDir, "plugins", "cache", marketplace, name, m.Identity.Version)
	for _, path := range []string{"hooks/hooks.json", "bin/style", ".codex-plugin/plugin.json"} {
		data, err := read(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(data, sourceFiles[path]) {
			return "", invalid
		}
	}
	return root, nil
}

func probeCacheComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\:\x00")
}
