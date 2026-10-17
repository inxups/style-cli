package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/pelletier/go-toml/v2"
	"github.com/speakeasy-api/agenthooks/install"
)

func nativeInstallFixture(t *testing.T, route string) (*Service, string, string, string, map[string][]byte) {
	requireFixtureRoute(t, route)
	t.Helper()
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "config")
	provider, hookName := adapter.ProviderID("codex"), "hooks.json"
	if route == "claude-user" {
		provider, hookName = "claude-code", "settings.json"
	}
	installation := adapter.Installation{Provider: provider, ConfigDir: config, Version: "2.1.315"}
	definition := codex.DefinitionForInstallation(installation)
	if provider == "claude-code" {
		definition = claude.DefinitionForInstallation(installation)
	}
	definition.Detector = fixedDetector{installation}
	otherInstallation := adapter.Installation{Provider: "claude-code", Version: "2.1.315", ConfigDir: filepath.Join(root, "other")}
	other := claude.DefinitionForInstallation(otherInstallation)
	if provider == "claude-code" {
		otherInstallation.Provider = "codex"
		other = codex.DefinitionForInstallation(otherInstallation)
	}
	other.Detector = fixedDetector{otherInstallation}
	service, err := New(definition, other)
	if err != nil {
		t.Fatal(err)
	}
	foreign := map[string][]byte{
		filepath.Join(config, hookName): []byte("{\n \"external\" : \"keep\", \"hooks\": {\"PreToolUse\":[{\"matcher\":\"Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"foreign --owner=other\"}]}]}\n}"),
	}
	if provider == "codex" {
		foreign[filepath.Join(config, "config.toml")] = []byte("# keep formatting\nmodel = 'external'")
	} else {
		foreign[filepath.Join(config, "CLAUDE.md")] = []byte("User-owned instructions.")
	}
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, data := range foreign {
		if err := os.WriteFile(path, data, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	source := writeInstallPackage(t, filepath.Join(root, "source"), nativePackageYAML("native-test"))
	return service, home, config, source, foreign
}

func nativePackageYAML(name string) string {
	return "spec: style/v0.3\nname: " + name + "\nversion: 0.1.0\ndescription: native test\nrules:\n" +
		"  - id: runtime\n    target: tool\n    tool: shell\n    at: before\n    text: Be careful.\n" +
		"  - id: later\n    priority: 80\n    target: native\n    codex: {event: PreToolUse, matcher: '^Bash$', command: 'printf later', timeout: 19}\n    claude-code: {event: PreToolUse, matcher: '*', command: 'printf later', timeout: 19}\n" +
		"  - id: earlier\n    priority: 20.5\n    target: native\n    codex: {event: PreToolUse, matcher: '^Bash$', command: 'printf earlier'}\n    claude-code: {event: PreToolUse, matcher: '*', command: 'printf earlier'}\n"
}

type nativeTestGroup struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type      string   `json:"type"`
		Command   string   `json:"command"`
		Args      []string `json:"args"`
		Timeout   int      `json:"timeout"`
		Async     bool     `json:"async"`
		OnFailure string   `json:"onFailure"`
	} `json:"hooks"`
}

func assertNativeInstalled(t *testing.T, home, config, route string, packages ...string) {
	t.Helper()
	path := filepath.Join(config, "hooks.json")
	if route == "codex-plugin" {
		path = filepath.Join(home, "adapters", "codex", "plugin", "hooks", "hooks.json")
	} else if route == "claude-user" {
		path = filepath.Join(config, "settings.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Hooks map[string][]nativeTestGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var tags []string
	nativeStarted := false
	trust, _ := os.ReadFile(filepath.Join(config, "config.toml"))
	var trustDocument struct {
		Hooks struct {
			State map[string]struct {
				TrustedHash string `toml:"trusted_hash"`
			} `toml:"state"`
		} `toml:"hooks"`
	}
	if route != "claude-user" {
		if err := toml.Unmarshal(trust, &trustDocument); err != nil {
			t.Fatal(err)
		}
	}
	for i, group := range document.Hooks["PreToolUse"] {
		if len(group.Hooks) != 1 {
			t.Fatalf("native groups merged: %s", data)
		}
		hook := group.Hooks[0]
		if !strings.HasPrefix(hook.Command, ": --owner=style --native=") {
			if nativeStarted {
				t.Fatal("runtime group follows native hook")
			}
			continue
		}
		nativeStarted = true
		tag, _, _ := strings.Cut(strings.TrimPrefix(hook.Command, ": --owner=style --native="), "; ")
		tags = append(tags, tag)
		expectedMatcher, timeout := "^Bash$", 60
		if route == "claude-user" {
			expectedMatcher = "*"
		}
		if strings.HasSuffix(tag, "/later") {
			timeout = 19
		}
		if group.Matcher != expectedMatcher || hook.Timeout != timeout {
			t.Fatalf("native matcher/timeout: %#v", group)
		}
		if hook.Async || hook.OnFailure != "" || len(hook.Args) != 0 || hook.Type != "command" {
			t.Fatalf("runner settings leaked to native: %#v", hook)
		}
		if route != "claude-user" {
			key := fmt.Sprintf(":pre_tool_use:%d:0", i)
			matched := false
			for actual, state := range trustDocument.Hooks.State {
				if strings.HasSuffix(actual, key) && state.TrustedHash == install.DefinitionHash("PreToolUse", group.Matcher, hook.Command, hook.Timeout) {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("native trust mismatch: key=%s config=%s", key, trust)
			}
		}
	}
	var expected []string
	for _, id := range []string{"earlier", "later"} {
		for _, name := range packages {
			expected = append(expected, name+"/"+id)
		}
	}
	if strings.Join(tags, ",") != strings.Join(expected, ",") {
		t.Fatalf("native order=%v want=%v; %s", tags, expected, data)
	}
	index, _ := os.ReadFile(filepath.Join(home, "build", "index.json"))
	if bytes.Contains(index, []byte("printf earlier")) || bytes.Contains(index, []byte("native-test/earlier")) {
		t.Fatal("native rule entered runtime index")
	}
}

func TestA3NativeInstallRoundTripAndRouteSwitch(t *testing.T) {
	requirePOSIXFixture(t, "author native install and POSIX route switch")
	for _, route := range []string{"codex-plugin", "codex-user", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			service, home, config, source, foreign := nativeInstallFixture(t, route)
			provider := "codex"
			if route == "claude-user" {
				provider = "claude-code"
			}
			request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{provider}, Routes: map[string]string{provider: route}}
			if route == "claude-user" {
				request.FailClosed = []string{provider}
			}
			routes := []string{route}
			if provider == "codex" {
				routes = append(routes, "codex-user", "codex-plugin")
			}
			for _, selected := range routes {
				request.Routes[provider] = selected
				if _, err := service.Install(context.Background(), request); err != nil {
					t.Fatalf("install: %#v", err)
				}
				assertNativeInstalled(t, home, config, selected, "native-test")
				repeated, err := service.Install(context.Background(), request)
				if err != nil || repeated.Changed {
					t.Fatalf("repeat install=%+v %v", repeated, err)
				}
				if selected == "codex-plugin" {
					data, _ := os.ReadFile(filepath.Join(config, "hooks.json"))
					if bytes.Contains(data, []byte("--owner=style")) {
						t.Fatal("user route residue")
					}
				} else if selected == "codex-user" {
					if _, err := os.Stat(filepath.Join(home, "adapters", "codex", "plugin", "hooks", "hooks.json")); !os.IsNotExist(err) {
						t.Fatalf("plugin residue: %v", err)
					}
				}
			}
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), nativePackageYAML("second"))
			request.Source = second
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			assertNativeInstalled(t, home, config, request.Routes[provider], "native-test", "second")
			if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
				t.Fatal(err)
			}
			assertNativeInstalled(t, home, config, request.Routes[provider], "second")
			if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			for path, before := range foreign {
				after, err := os.ReadFile(path)
				info, statErr := os.Stat(path)
				if err != nil || !bytes.Equal(before, after) || statErr != nil || info.Mode().Perm() != 0o640 {
					t.Errorf("foreign file changed: %s data=%q err=%v", path, after, err)
				}
			}
		})
	}
}
