package install

import (
	"encoding/json"
	"io/fs"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

func TestRawNativeCommandsBypassRunnerAndPreserveShellContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native command ownership uses a POSIX no-op")
	}
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			provider := agenthooks.ProviderCodex
			target := Target{Provider: provider, Scope: ScopeUser, Dir: t.TempDir(), ConfigDir: t.TempDir()}
			path := "hooks.json"
			if route == "codex-plugin" {
				target.Scope, path = ScopePlugin, "hooks/hooks.json"
			} else if route == "claude-user" {
				provider, target.Provider, path = agenthooks.ProviderClaudeCode, agenthooks.ProviderClaudeCode, "settings.json"
			}
			matcher := "Bash"
			if provider == agenthooks.ProviderClaudeCode {
				matcher = "*"
			}
			manifest := Manifest{Command: []string{"/never-run/style", "--never-use"}, Owner: "style", Fail: agenthooks.FailClosed,
				Identity: Identity{Name: "style-runtime", PluginID: "style-runtime@style"},
				Hooks: []HookSpec{{Kind: agenthooks.KindToolPre, NativeEvent: "PreToolUse", NativeMatcher: matcher,
					Timeout: 17 * time.Second, RawCommand: "cat; exit 7", RawTag: "pkg/shell' $tag"}}}
			// A native hook must ignore fail-closed and work even when the
			// harness version is unknown; it never invokes the STYLE runner.
			rendered, err := Render(manifest, target)
			if err != nil {
				t.Fatal(err)
			}
			data, err := fs.ReadFile(rendered, path)
			if err != nil {
				t.Fatal(err)
			}
			var document struct {
				Hooks map[string][]claudeMatcherEntry `json:"hooks"`
			}
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			entry := document.Hooks["PreToolUse"][0]
			hook := entry.Hooks[0]
			if entry.Matcher != matcher || hook.Timeout != 17 || hook.Async || hook.OnFailure != "" || len(hook.Args) != 0 {
				t.Fatalf("native fields changed: %#v", entry)
			}
			for _, forbidden := range []string{"agenthooks", "--async", "--filter", "--fail-closed", "$PLUGIN_ROOT", "/never-run"} {
				if strings.Contains(hook.Command, forbidden) {
					t.Fatalf("native command contains %q: %s", forbidden, hook.Command)
				}
			}
			input := "{\"event\":\"original stdin\"}\n"
			command := exec.Command("sh", "-c", hook.Command)
			command.Stdin = strings.NewReader(input)
			output, err := command.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 || string(output) != input {
				t.Fatalf("stdin/exit changed: output=%q error=%v", output, err)
			}
			if provider == agenthooks.ProviderCodex {
				trust, err := fs.ReadFile(rendered, "config.toml")
				if err != nil || !strings.Contains(string(trust), DefinitionHash("PreToolUse", matcher, hook.Command, 17)) {
					t.Fatalf("native trust does not match command: %s %v", trust, err)
				}
			}
		})
	}
}

func TestCodexRawNativeEventsAndSessionEndTimeout(t *testing.T) {
	requirePOSIXFixture(t, "POSIX plugin/project or author native fixture")
	manifest := Manifest{Command: []string{"style"}, Owner: "style"}
	for event := range codexEventLabel {
		manifest.Hooks = append(manifest.Hooks, HookSpec{NativeEvent: event, RawCommand: "true", RawTag: "pkg/" + event, Timeout: time.Minute})
	}
	rendered, err := Render(manifest, Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := fs.ReadFile(rendered, "hooks.json")
	var document struct {
		Hooks map[string][]claudeMatcherEntry `json:"hooks"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Hooks) != len(codexEventLabel) || document.Hooks["SessionEnd"][0].Hooks[0].Timeout != 3 {
		t.Fatalf("native events/timeout: %s", data)
	}
}
