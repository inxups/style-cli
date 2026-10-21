package install

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

// codexMarketplaceFixture returns a Codex plugin target whose plugin
// directory sits inside a local marketplace root, plus its rendered files.
func codexMarketplaceFixture(t *testing.T) (Manifest, Target, fs.FS) {
	t.Helper()
	requirePOSIXFixture(t, "POSIX plugin route")
	marketplace := t.TempDir()
	config := t.TempDir()
	m := Manifest{Command: []string{"/fixture/style"}, Owner: "style", Identity: Identity{Name: "style-runtime", Version: "0.1.0", PluginID: "style-runtime@style"}, Hooks: []HookSpec{{Kind: agenthooks.KindToolPre, NativeMatcher: "Bash", Blocking: true, Timeout: 10 * time.Second}}}
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopePlugin, Dir: filepath.Join(marketplace, "plugin"), ConfigDir: config, LocalMarketplaceName: "style", LocalMarketplaceDir: marketplace}
	rendered, err := Render(m, target)
	if err != nil {
		t.Fatal(err)
	}
	return m, target, rendered
}

func TestCodexPluginRendersLocalMarketplace(t *testing.T) {
	m, target, rendered := codexMarketplaceFixture(t)
	data, err := fs.ReadFile(rendered, codexMarketplacePath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string            `json:"name"`
			Source map[string]string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Name != "style" || len(catalog.Plugins) != 1 || catalog.Plugins[0].Name != m.Identity.Name || catalog.Plugins[0].Source["source"] != "local" || catalog.Plugins[0].Source["path"] != "./plugin" {
		t.Fatalf("catalog = %s", data)
	}
	if root := renderedRoot(target, codexMarketplacePath); root != target.LocalMarketplaceDir {
		t.Fatalf("catalog root = %s", root)
	}
	if root := renderedRoot(target, "hooks/hooks.json"); root != target.Dir {
		t.Fatalf("plugin root = %s", root)
	}

	nested := target
	nested.Dir = filepath.Join(target.LocalMarketplaceDir, "plugins", "plugin")
	if _, err := Render(m, nested); err == nil {
		t.Fatal("rendered a catalog whose plugin is not a direct child of the marketplace")
	}
	renamed := target
	renamed.LocalMarketplaceName = "other"
	if _, err := Render(m, renamed); err == nil {
		t.Fatal("rendered a catalog that does not match the plugin id")
	}
	unmanaged := target
	unmanaged.LocalMarketplaceName, unmanaged.LocalMarketplaceDir = "", ""
	plain, err := Render(m, unmanaged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(plain, codexMarketplacePath); err == nil {
		t.Fatal("rendered a catalog without a local marketplace")
	}
}

func TestInspectCodexRegistration(t *testing.T) {
	for _, mode := range []string{"no-config", "unregistered", "marketplace-only", "disabled", "stale-cache", "missing-cache", "enabled", "foreign-source", "remote-source", "bad-config"} {
		t.Run(mode, func(t *testing.T) {
			m, target, rendered := codexMarketplaceFixture(t)
			files := map[string][]byte{}
			config := filepath.Join(target.ConfigDir, "config.toml")
			source, _ := json.Marshal(target.LocalMarketplaceDir)
			marketplace := "\n[marketplaces.style]\nsource_type = \"local\"\nsource = " + string(source) + "\n"
			plugin := "\n[plugins.\"style-runtime@style\"]\nenabled = true\n"
			switch mode {
			case "no-config":
			case "unregistered":
				files[config] = []byte("model = \"fixture\"\n")
			case "marketplace-only":
				files[config] = []byte(marketplace)
			case "disabled":
				files[config] = []byte(marketplace + strings.Replace(plugin, "true", "false", 1))
			case "foreign-source":
				other, _ := json.Marshal(t.TempDir())
				files[config] = []byte("\n[marketplaces.style]\nsource_type = \"local\"\nsource = " + string(other) + "\n" + plugin)
			case "remote-source":
				files[config] = []byte(strings.Replace(marketplace, "local", "git", 1) + plugin)
			case "bad-config":
				files[config] = []byte("[plugins")
			default:
				files[config] = []byte(marketplace + plugin)
			}
			if mode == "stale-cache" || mode == "enabled" {
				cache := filepath.Join(target.ConfigDir, "plugins", "cache", "style", "style-runtime", "0.1.0")
				for _, path := range codexCacheArtifacts {
					data, err := fs.ReadFile(rendered, path)
					if err != nil {
						t.Fatal(err)
					}
					if mode == "stale-cache" && path == "bin/style" {
						data = append(data, '\n')
					}
					files[filepath.Join(cache, filepath.FromSlash(path))] = data
				}
			}
			read := func(path string) ([]byte, error) {
				if data, ok := files[path]; ok {
					return data, nil
				}
				return nil, os.ErrNotExist
			}
			got, err := InspectCodexRegistration(m, target, read)
			want := map[string]struct {
				state               string
				registered, enabled bool
				fails               bool
			}{
				"no-config":        {state: "registration-required"},
				"unregistered":     {state: "registration-required"},
				"marketplace-only": {state: "registration-required"},
				"disabled":         {state: "disabled", registered: true},
				"stale-cache":      {state: "refresh-required", registered: true, enabled: true},
				"missing-cache":    {state: "refresh-required", registered: true, enabled: true},
				"enabled":          {state: "enabled", registered: true, enabled: true},
				"foreign-source":   {state: "unknown", fails: true},
				"remote-source":    {state: "unknown", fails: true},
				"bad-config":       {state: "unknown", fails: true},
			}[mode]
			if (err != nil) != want.fails || got.State != want.state || got.Registered != want.registered || got.Enabled != want.enabled {
				t.Fatalf("registration = %+v err=%v, want %+v", got, err, want)
			}
			if len(got.Commands) != 4 || !strings.Contains(got.Commands[0], "codex plugin marketplace add "+shellQuote(target.LocalMarketplaceDir)) || !strings.Contains(got.Commands[1], "codex plugin add style-runtime@style") || !strings.HasPrefix(got.Commands[0], "CODEX_HOME="+shellQuote(target.ConfigDir)+" ") {
				t.Fatalf("commands = %q", got.Commands)
			}
		})
	}
}
