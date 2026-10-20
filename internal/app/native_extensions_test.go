package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestM3A16SixRouteNativeExtensionsRejectBeforeWrites(t *testing.T) {
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
				config = root
			} else {
				fixture := route
				if route == "claude-plugin" {
					fixture = "claude-user"
				}
				s, home, config, source, _ = nativeInstallFixture(t, fixture)
			}
			t.Setenv("STYLE_HOME", home)
			marker := filepath.Join(t.TempDir(), "native-must-not-execute")
			manifest := fmt.Sprintf("spec: style/v0.3\nname: extensions\nversion: 1.0.0\ndescription: native boundary fixture\nnative_hooks:\n  - {id: native, %s: {event: Stop, command: 'touch %s'}}\n", harness, marker)
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
			installed, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if route == "claude-plugin" {
				a8Registration(t, installed.Registration[0], config, "enabled")
			}
			managed := filepath.Join(home, "packages", "extensions", "STYLE.yaml")
			candidate := filepath.Join(t.TempDir(), "candidate")
			roots := []string{home, source, config, candidate}
			for _, field := range []string{"async: true", "onFailure: block", "type: http"} {
				invalid := strings.Replace(manifest, "event: Stop,", "event: Stop, "+field+",", 1)
				writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), invalid)
				writeSyncFile(t, managed, invalid)
				writeSyncFile(t, filepath.Join(candidate, "STYLE.yaml"), strings.Replace(invalid, "name: extensions", "name: candidate", 1))

				commands := [][]string{{"validate", source, "--json"}, {"plan", source, "--harness=" + harness, "--json"}, {"install", candidate, "--harness=" + harness, "--route=" + harness + "=" + route, "--json"}, {"update", "--all", "--json"}, {"sync", "--json"}, {"sync", "extensions", "--json"}}
				for _, command := range commands {
					if command[0] == "sync" {
						writeSyncFile(t, managed, invalid)
					} else {
						writeSyncFile(t, managed, manifest)
					}
					before := installedTree(t, roots...)
					var out, logs bytes.Buffer
					code := RunWithServiceInput(context.Background(), s, source, command, approve(), &out, &logs)
					expectedCode := 1
					expectedDiagnostic := "not supported;"
					if command[0] == "update" {
						expectedDiagnostic = "source-invalid"
					}
					if code != expectedCode || !strings.Contains(out.String()+logs.String(), expectedDiagnostic) {
						t.Fatal(field, command, code, out.String(), logs.String())
					}
					if !reflect.DeepEqual(before, installedTree(t, roots...)) {
						t.Fatal(field, command, "changed files")
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatal("native command executed", err)
					}
				}
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
			writeSyncFile(t, managed, manifest)
			lock := a5Lock(t, home)
			a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
			if _, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
				t.Fatal(err)
			}
			lock()
			if route == "claude-plugin" {
				a8Registration(t, installed.Registration[0], config, "disabled")
			}
			if _, err = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("lifecycle executed author command", err)
			}
		})
	}
}
