package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
)

const modelObserverManifest = `spec: style/v0.3
name: model-observer
version: 0.1.0
description: model switch installation regression
reads: {model: true}
rules:
  - id: session
    target: system
    text: 'Current model: {{model}}.'
`

type claudeHookSettings struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Args  []string `json:"args"`
			Async bool     `json:"async"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func installClaudeModelFixture(t *testing.T, manifest string) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "claude")
	installation := adapter.Installation{Provider: "claude-code", Version: "2.1.300", ConfigDir: config}
	definition := claude.DefinitionForInstallation(installation)
	definition.Detector = fixedDetector{installation}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	source := writeInstallPackage(t, filepath.Join(root, "source"), manifest)
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Yes: true}); err != nil {
		t.Fatal(err)
	}
	return service, home, config
}

func readClaudeHookSettings(t *testing.T, config string) claudeHookSettings {
	t.Helper()
	var settings claudeHookSettings
	data, err := os.ReadFile(filepath.Join(config, "settings.json"))
	if os.IsNotExist(err) {
		return settings
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestInstallClaudeModelSwitchRefreshesNextPrompt(t *testing.T) {
	t.Setenv("CLAUDE_EFFORT", "")
	service, home, config := installClaudeModelFixture(t, modelObserverManifest)
	settings := readClaudeHookSettings(t, config)
	invoke := func(event, payload string) string {
		t.Helper()
		groups := settings.Hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("installed %s subscription = %#v; expected one runtime hook", event, groups)
		}
		if event == "PostModelSwitch" && groups[0].Hooks[0].Async {
			t.Fatal("model observer must finish its state write before the next prompt")
		}
		var stdout, stderr bytes.Buffer
		code := RunWithInput(context.Background(), groups[0].Hooks[0].Args, strings.NewReader(payload), &stdout, &stderr)
		if code != ExitOK || strings.Contains(stderr.String(), "ERROR") {
			t.Fatalf("%s code=%d stderr=%q", event, code, stderr.String())
		}
		return stdout.String()
	}
	if got := invoke("SessionStart", `{"session_id":"model-switch","hook_event_name":"SessionStart","source":"startup","model":"claude-sonnet-5-5"}`); !strings.Contains(got, "Current model: claude-sonnet-5-5.") {
		t.Fatalf("startup output = %q", got)
	}
	if got := invoke("PostModelSwitch", `{"session_id":"model-switch","hook_event_name":"PostModelSwitch","from_model":"claude-sonnet-5-5","to_model":"claude-haiku-5-5"}`); strings.TrimSpace(got) != "{}" {
		t.Fatalf("observe-only model switch returned non-neutral output %q", got)
	}
	// Prompt payload deliberately omits model: it must use the switch event's
	// persisted value rather than retaining the startup model.
	prompt := `{"session_id":"model-switch","hook_event_name":"UserPromptSubmit","prompt":"Quote the current model."}`
	if got := invoke("UserPromptSubmit", prompt); !strings.Contains(got, "Current model: claude-haiku-5-5.") || strings.Contains(got, "claude-sonnet-5-5") {
		t.Fatalf("first prompt after switch = %q", got)
	}
	if got := invoke("UserPromptSubmit", prompt); strings.TrimSpace(got) != "{}" {
		t.Fatalf("second prompt repeated model context: %q", got)
	}
	// Removing the final model dependency must remove its observer as part of
	// the normal trust/recompile lifecycle.
	managed := filepath.Join(home, "packages", "model-observer", "STYLE.yaml")
	writeSyncFile(t, managed, strings.ReplaceAll(modelObserverManifest, "Current model: {{model}}.", "Keep responses concise."))
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "model-observer", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeHookSettings(t, config).Hooks["PostModelSwitch"]; len(got) != 0 {
		t.Fatalf("model observer remained without model-dependent rules: %#v", got)
	}
}

func TestInstallClaudeModelObserverFollowsRuleRequirements(t *testing.T) {
	for _, test := range []struct {
		name, rules string
		want        bool
	}{
		{"system template", "  - id: session\n    target: system\n    text: 'Current model: {{model}}.'\n", true},
		{"tool template", "  - id: edit\n    target: tool\n    tool: shell\n    text: 'Current model: {{model}}.'\n", true},
		{"model condition", "  - id: edit\n    target: tool\n    tool: shell\n    text: Review the command.\n    when: {model: 'claude-*'}\n", true},
		{"static only", "  - id: static\n    target: system\n    text: Keep responses concise.\n", false},
		{"unrelated tool", "  - id: edit\n    target: tool\n    tool: shell\n    text: Review the command.\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			header, _, _ := strings.Cut(modelObserverManifest, "rules:\n")
			manifest := header + "rules:\n" + test.rules
			_, _, config := installClaudeModelFixture(t, manifest)
			got := len(readClaudeHookSettings(t, config).Hooks["PostModelSwitch"]) != 0
			if got != test.want {
				t.Fatalf("model switch subscription = %v, want %v", got, test.want)
			}
		})
	}
}
