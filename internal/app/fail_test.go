package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/inxups/style-cli/core/compile"
	"path/filepath"
	"strings"
	"testing"
)

func a14Manifest() string {
	return "spec: style/v0.3\nname: missing-data\nversion: 1.0.0\ndescription: failure guard fixture\nrules:\n  - {id: r, target: event, event: tool.pre, fail: closed, text: Guarded rule}\n"
}
func TestM3A14SixRouteFailLifecycleAndMarkerRemoval(t *testing.T) {
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
				fixture := route
				if route == "claude-plugin" {
					fixture = "claude-user"
				}
				s, home, config, source, _ = nativeInstallFixture(t, fixture)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a14Manifest())
			installed, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if route == "claude-plugin" {
				a8Registration(t, installed.Registration[0], config, "enabled")
			}
			lock, err := s.loadLock(home)
			if err != nil || lock.MinIndexFormat != 7 || lock.Format < 2 {
				t.Fatal(lock, err)
			}
			data := readSyncFile(t, filepath.Join(home, "build/index.json"))
			var index compile.Index
			if err = json.Unmarshal(data, &index); err != nil || index.Format != 7 {
				t.Fatal(index.Format, err)
			}
			if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			if r, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			check := a5Lock(t, home)
			a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
			if _, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
				t.Fatal(err)
			}
			check()
			if !bytes.Equal(data, readSyncFile(t, filepath.Join(home, "build/index.json"))) {
				t.Fatal("frozen changed failure policy index")
			}
			d, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
			if err != nil || !d.Healthy {
				t.Fatal(d.Diagnostics, err)
			}
			found := false
			for _, v := range d.Diagnostics {
				found = found || v.Code == "runtime-index-7"
			}
			if !found {
				t.Fatal(d.Diagnostics)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.Replace(a14Manifest(), "fail: closed, ", "", 1))
			if _, err = s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			lock, err = s.loadLock(home)
			if err != nil || lock.MinIndexFormat != 0 || bytes.Contains(readSyncFile(t, filepath.Join(home, "style.lock.yaml")), []byte("min_index_format")) {
				t.Fatal(lock, err)
			}
			if route == "claude-plugin" {
				a8Registration(t, installed.Registration[0], config, "disabled")
			}
			if _, err = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestM3A14CloudCarriesFailurePolicy(t *testing.T) {
	s, _, root, source := a7Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a14Manifest())
	if _, err := s.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a7Export(t, s)
	_, files, err := readCloud(root)
	if err != nil {
		t.Fatal(err)
	}
	var index compile.Index
	if err = json.Unmarshal(files[cloudDir+"/index.json"], &index); err != nil || index.Format != 7 {
		t.Fatal(index, err)
	}
	found := false
	for _, entry := range index.Harnesses["claude-code"]["tool.pre"] {
		found = found || entry.Fail == "closed"
	}
	if !found {
		t.Fatal("cloud lost policy")
	}
}
