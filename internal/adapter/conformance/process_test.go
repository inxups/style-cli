package conformance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/inxups/style-cli/internal/app"
	"github.com/speakeasy-api/agenthooks"
)

func buildBinary(t testing.TB) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "style with spaces"+fixtureExeSuffix())
	command := exec.Command("go", "build", "-o", binary, "../../../cmd/style")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	return binary
}

func processEnv(root string) []string {
	result := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "HOME", "STYLE_HOME", "CODEX_HOME", "CODEX_SANDBOX", "CLAUDE_CONFIG_DIR", "CLAUDE_EFFORT":
			continue
		}
		result = append(result, entry)
	}
	// A deliberately different STYLE_HOME proves the persisted argv wins.
	return append(result, "HOME="+filepath.Join(root, "user"), "STYLE_HOME="+filepath.Join(root, "wrong-style-home"), "CODEX_HOME="+filepath.Join(root, "codex"), "CLAUDE_CONFIG_DIR="+filepath.Join(root, "claude"))
}

func writeIndexOnlyHome(t *testing.T, home string, index compile.Index) {
	t.Helper()
	raw, err := compile.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "build"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "build", "index.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"packages/broken/STYLE.yaml", "style.lock.yaml", "settings/broken.yaml"} {
		destination := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, []byte("must not be parsed: ["), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func hookArgs(home string, suite providerSuite) []string {
	return []string{"--style-home=" + home, "--harness-home=" + filepath.Join(home, "harness"), "--harness-version=test-version", "agenthooks", "run", "--provider=" + string(suite.id), "--owner=style"}
}

func TestBuiltCLIHookMatchesEveryWireGoldenWithoutSources(t *testing.T) {
	binary := buildBinary(t)
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			home := t.TempDir()
			writeIndexOnlyHome(t, home, buildIndex(t, suite, read(t, filepath.Join(corpus, "STYLE.yaml"))))
			paths, err := filepath.Glob(filepath.Join(corpus, string(suite.id), "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range paths {
				t.Run(filepath.Base(path), func(t *testing.T) {
					payload := read(t, path)
					var event struct {
						SessionID string `json:"session_id"`
					}
					if err := json.Unmarshal(payload, &event); err != nil {
						t.Fatal(err)
					}
					store, err := state.New(filepath.Join(home, "state"))
					if err != nil {
						t.Fatal(err)
					}
					// Each fixture gets fresh scope; resolved effort prevents any
					// first-session configuration lookup in this hot-path test.
					if _, _, err := store.Update(event.SessionID, "", state.EventUpdate{Source: "clear", EffortResolved: true}); err != nil {
						t.Fatal(err)
					}
					command := exec.Command(binary, hookArgs(home, suite)...)
					command.Env, command.Stdin = processEnv(home), bytes.NewReader(payload)
					var stdout, stderr bytes.Buffer
					command.Stdout, command.Stderr = &stdout, &stderr
					if err := command.Run(); err != nil || strings.Contains(stderr.String(), "level=ERROR") {
						t.Fatalf("CLI hook failed: %v %s", err, stderr.String())
					}
					want := read(t, strings.TrimSuffix(path, ".json")+".stdout.golden")
					if !bytes.Equal(stdout.Bytes(), want) {
						t.Fatalf("process output=%s want=%s", stdout.Bytes(), want)
					}
				})
			}
			if err := os.WriteFile(filepath.Join(home, "build", "index.json"), []byte("broken index"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, closed := range []bool{false, true} {
				if closed && suite.id != agenthooks.ProviderClaudeCode {
					continue
				}
				args := hookArgs(home, suite)
				if closed {
					args = append(args, "--fail-closed")
				}
				command := exec.Command(binary, args...)
				command.Env, command.Stdin = processEnv(home), bytes.NewReader(read(t, filepath.Join(corpus, string(suite.id), "deny_tool.json")))
				var stdout, stderr bytes.Buffer
				command.Stdout, command.Stderr = &stdout, &stderr
				err := command.Run()
				if !strings.Contains(stderr.String(), "index-invalid") {
					t.Fatalf("missing safe diagnostic: %s", stderr.String())
				}
				if closed {
					if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || !strings.Contains(stdout.String(), `"permissionDecision":"deny"`) {
						t.Fatalf("fail-closed %v %s", err, stdout.String())
					}
				} else if err != nil || suite.id == agenthooks.ProviderCodex && stdout.Len() != 0 || suite.id == agenthooks.ProviderClaudeCode && stdout.String() != "{}" {
					t.Fatalf("fail-open %v %s", err, stdout.String())
				}
			}
		})
	}
}

type fixedInstallation struct{ value adapter.Installation }

func (d fixedInstallation) Detect(context.Context) (adapter.Installation, bool, error) {
	return d.value, true, nil
}

func TestGeneratedRoutesInvokeBuiltCLIWithPersistedHome(t *testing.T) {
	binary := buildBinary(t)
	for _, name := range []string{"codex-plugin", "codex-user", "claude-user"} {
		t.Run(name, func(t *testing.T) {
			if runtime.GOOS == "windows" && name == "codex-plugin" {
				t.Skip("D11: POSIX plugin launcher")
			}
			root := t.TempDir()
			home, config := filepath.Join(root, "style home"), filepath.Join(root, "config home")
			installation := adapter.Installation{Provider: agenthooks.ProviderCodex, Version: "0.159.3", ConfigDir: config}
			definition := codex.DefinitionForInstallation(installation)
			plugin := filepath.Join(home, "adapters", "codex", "plugin")
			definition.Routes = []adapter.Route{codex.NewPluginRouteForCommand(plugin, config, binary, nil), codex.NewUserRouteForCommand(config, binary, nil)}
			configPath := filepath.Join(config, "hooks.json")
			if name == "codex-plugin" {
				configPath = filepath.Join(plugin, "hooks", "hooks.json")
			}
			if name == "claude-user" {
				installation.Provider, installation.Version = agenthooks.ProviderClaudeCode, "2.1.315"
				definition = claude.DefinitionForInstallation(installation)
				definition.Routes = []adapter.Route{claude.NewUserRouteForInstallation(config, installation.Version, binary, nil)}
				configPath = filepath.Join(config, "settings.json")
			}
			definition.Detector = fixedInstallation{installation}
			service, err := app.New(definition)
			if err != nil {
				t.Fatal(err)
			}
			source, err := filepath.Abs(corpus)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Install(context.Background(), app.InstallRequest{Source: source, Home: home, Harnesses: []string{string(installation.Provider)}, Routes: map[string]string{string(installation.Provider): name}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			var configJSON struct {
				Hooks map[string][]struct {
					Hooks []struct {
						Command string   `json:"command"`
						Args    []string `json:"args"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(read(t, configPath), &configJSON); err != nil {
				t.Fatal(err)
			}
			commands := configJSON.Hooks["PreToolUse"]
			if len(commands) == 0 || len(commands[0].Hooks) == 0 {
				t.Fatal("missing generated PreToolUse")
			}
			hook := commands[0].Hooks[0]
			var command *exec.Cmd
			if installation.Provider == agenthooks.ProviderCodex {
				if runtime.GOOS == "windows" {
					command = exec.Command("cmd.exe", "/d", "/s", "/c", hook.Command)
				} else {
					command = exec.Command("/bin/sh", "-c", hook.Command)
				}
			} else {
				command = exec.Command(hook.Command, hook.Args...)
			}
			command.Env = append(processEnv(root), "PLUGIN_ROOT="+plugin)
			command.Stdin = bytes.NewReader(read(t, filepath.Join(corpus, string(installation.Provider), "deny_tool.json")))
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil || strings.Contains(stderr.String(), "level=ERROR") {
				t.Fatalf("generated command failed: %v %s", err, stderr.String())
			}
			if want := read(t, filepath.Join(corpus, string(installation.Provider), "deny_tool.stdout.golden")); !bytes.Equal(stdout.Bytes(), want) {
				t.Fatalf("generated route output=%s want=%s", stdout.Bytes(), want)
			}
		})
	}
}

func fixtureExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
