package codex

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRemovePluginRegistrationPreservesOtherConfig(t *testing.T) {
	root := t.TempDir()
	r := NewPluginRouteForCommand(filepath.Join(root, "plugin"), filepath.Join(root, "codex"), "/tmp/style", nil).(*route)
	market := "[marketplaces.style]\nsource_type = 'local'\nsource = " + strconv.Quote(root) + "\n"
	plugin := "[plugins.\"style-runtime@style\"]\nenabled = true\n"
	foreign := "# keep user bytes\nmodel = 'user-model'\nnote = '''\n[plugins.\"style-runtime@style\"]\nenabled = true\n'''\n\n"
	other := "# Other plugin\n[plugins.\"keep@other\"]\nenabled = true\n\n[marketplaces.other]\nsource = '/other'\n"
	for _, test := range []struct {
		name, owned, keep string
	}{
		{"tables", market + plugin, other},
		{"nested-tables", market + plugin + "[plugins.\"style-runtime@style\".options]\nmode = 'owned'\n", other},
		{"dotted-keys", "marketplaces.style.source_type = 'local'\nmarketplaces.style.source = " + strconv.Quote(root) + "\nplugins.\"style-runtime@style\".enabled = false\n", other},
		{"inline-entry", "[marketplaces]\nstyle = { source_type = 'local', source = " + strconv.Quote(root) + " }\n[plugins]\n\"style-runtime@style\" = { enabled = true }\n", other},
		{"orphan-plugin", plugin, other},
		{"shared-marketplace", plugin, market + "[plugins.\"keep@style\"]\nenabled = true\n" + other},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := r.removePluginRegistration([]byte(foreign + test.owned + test.keep))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(got, []byte(foreign)) || !bytes.HasSuffix(got, []byte(test.keep)) {
				t.Fatalf("unrelated bytes changed: %s", got)
			}
			config, err := parseStaticTOML(got)
			if err != nil {
				t.Fatal(err)
			}
			if plugins, _ := config["plugins"].(map[string]any); plugins[runtimePluginID] != nil {
				t.Fatalf("runtime registration remains: %s", got)
			}
		})
	}
}

func TestRemovePluginRegistrationRejectsDifferentInstallation(t *testing.T) {
	r := NewPluginRouteForCommand(filepath.Join(t.TempDir(), "plugin"), t.TempDir(), "/tmp/style", nil).(*route)
	for _, data := range []string{
		"[marketplaces.style]\nsource_type = 'local'\nsource = '/another/style'\n",
		"[marketplaces.style]\nsource_type = 'git'\nsource = 'https://example.com/style.git'\n",
	} {
		if _, err := r.removePluginRegistration([]byte(data)); err == nil || !strings.Contains(err.Error(), "does not belong") {
			t.Fatalf("foreign marketplace accepted: %v", err)
		}
	}
}
