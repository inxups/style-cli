package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
)

func a10Manifest() string {
	return "spec: style/v0.3\nname: event-content\nversion: 1.0.0\ndescription: event fixture\nreads: {event: [tool.input.command]}\nrules:\n  - {id: r, target: event, event: tool.pre, text: 'Review {{event.tool.input.command}}'}\n"
}
func TestM3A10SixRouteLifecycleAndFrozen(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user", "claude-plugin", "codex-project", "claude-project"} {
		t.Run(route, func(t *testing.T) {
			requireFixtureRoute(t, route)
			harness := "codex"
			if strings.HasPrefix(route, "claude-") {
				harness = "claude-code"
			}
			var s *Service
			var home, source, config string
			if strings.HasSuffix(route, "-project") {
				var base, root string
				s, base, root, source = a6Setup(t)
				s = a6Scope(t, s, root, base)
				home = s.project.Home
			} else {
				fixtureRoute := route
				if route == "claude-plugin" {
					fixtureRoute = "claude-user"
				}
				s, home, config, source, _ = nativeInstallFixture(t, fixtureRoute)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a10Manifest())
			result, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if route == "claude-plugin" {
				a8Registration(t, result.Registration[0], config, "enabled")
			}
			data := readSyncFile(t, filepath.Join(home, "build/index.json"))
			var index compile.Index
			if err := json.Unmarshal(data, &index); err != nil || index.Format != 3 {
				t.Fatal(err, string(data))
			}
			a1AssertProvenance(t, home)
			if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			if r, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			d, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
			if err != nil || !d.Healthy {
				t.Fatal(d.Diagnostics, err)
			}
			found := false
			for _, v := range d.Diagnostics {
				found = found || v.Code == "event-index-3"
			}
			if !found {
				t.Fatal(d.Diagnostics)
			}
			check := a5Lock(t, home)
			a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
			if _, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
				t.Fatal(err)
			}
			check()
			if !bytes.Equal(data, readSyncFile(t, filepath.Join(home, "build/index.json"))) {
				t.Fatal("frozen changed event index")
			}
			if route == "claude-plugin" {
				a8Registration(t, result.Registration[0], config, "disabled")
			}
			if _, err = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestM3A10CloudBundleRetainsFencedIndex(t *testing.T) {
	s, base, root, source := a7Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a10Manifest())
	if _, err := s.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a7Export(t, s)
	_, files, err := readCloud(root)
	if err != nil {
		t.Fatal(err)
	}
	var index compile.Index
	if err = json.Unmarshal(files[cloudDir+"/index.json"], &index); err != nil || index.Format != 3 {
		t.Fatal(index.Format, err)
	}
	if bytes.Contains(files[cloudDir+"/index.json"], []byte(base)) {
		t.Fatal("private path leaked")
	}
}
