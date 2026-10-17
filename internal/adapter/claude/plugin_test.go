package claude

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
)

func TestM3A8PluginProbeIdentityAndRegistration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("D11: POSIX plugin route unavailable on Windows")
	}
	root := t.TempDir()
	config := filepath.Join(root, "claude")
	home := filepath.Join(root, "style")
	r := NewPluginRouteForInstallation(config, "2.1.301", "/fixed/style", nil).(*route).BindStorage(home).(*route)
	planned, err := r.Plan(context.Background(), DefaultSurface(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), planned); err != nil {
		t.Fatal(err)
	}
	if _, err = r.InspectProbe(context.Background(), os.ReadFile); err == nil {
		t.Fatal("unregistered probe accepted")
	}
	registration, err := r.Registration(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	put := func(name string, v any) {
		b, _ := json.Marshal(v)
		path := filepath.Join(config, name)
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	market := r.marketplaceName()
	put("plugins/known_marketplaces.json", map[string]any{market: map[string]any{"source": map[string]string{"source": "directory", "path": r.marketplaceRoot()}, "installLocation": r.marketplaceRoot()}})
	put("plugins/installed_plugins.json", map[string]any{"version": 2, "plugins": map[string]any{registration.Plugin: []any{map[string]string{"scope": "user", "version": "0.1.0"}}}})
	put("settings.json", map[string]any{"enabledPlugins": map[string]bool{registration.Plugin: true}})
	inspected, err := r.InspectProbe(context.Background(), os.ReadFile)
	if err != nil || inspected.Root != r.pluginRoot() || inspected.ConfigRoot != config || len(inspected.Events) == 0 {
		t.Fatal(inspected, err)
	}
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(r.pluginRoot())))
	if !r.VerifyProbeOrigin(adapter.ProbeOrigin{PluginRootSHA256: hash, RouteRootSHA256: hash}) || r.VerifyProbeOrigin(adapter.ProbeOrigin{}) || r.VerifyProbeOrigin(adapter.ProbeOrigin{PluginRootSHA256: "wrong"}) {
		t.Fatal("plugin origin binding")
	}
}
