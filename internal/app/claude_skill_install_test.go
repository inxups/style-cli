package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInstallClaudeSkillSharesDedupAcrossEntrypoints(t *testing.T) {
	t.Setenv("CLAUDE_EFFORT", "")
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, at := range []string{"before", "after"} {
		for _, explicitFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/explicit-first=%t", at, explicitFirst), func(t *testing.T) {
				manifest := `spec: style/v0.3
name: skill-install
version: 0.1.0
description: installed skill subscription regression
rules:
  - id: review
    target: skill
    skill: code-review
    at: ` + at + `
    text: M1B installed skill limit.
`
				service, home, config := installClaudeFixture(t, manifest)
				settings := readClaudeHookSettings(t, config)
				implicit := "PostToolUse"
				if at == "before" {
					implicit = "PreToolUse"
				}
				invoke := func(event string) string {
					t.Helper()
					selector := "Skill"
					if event == "UserPromptExpansion" {
						selector = "code-review"
					}
					var args []string
					for _, group := range settings.Hooks[event] {
						matched, err := regexp.MatchString(group.Matcher, selector)
						if err != nil {
							t.Fatal(err)
						}
						if !matched {
							continue
						}
						if len(group.Hooks) != 1 || args != nil {
							t.Fatalf("expected one matching %s subscription: %#v", event, settings.Hooks[event])
						}
						args = group.Hooks[0].Args
					}
					if args == nil {
						t.Fatalf("installed %s hooks do not match %s: %#v", event, selector, settings.Hooks[event])
					}
					payload := fmt.Sprintf(`{"session_id":"skill-shared","hook_event_name":%q,"tool_name":"Skill","tool_input":{"skill":"code-review"},"tool_response":"loaded"}`, event)
					if event == "UserPromptExpansion" {
						payload = `{"session_id":"skill-shared","hook_event_name":"UserPromptExpansion","expansion_type":"slash_command","command_name":"code-review","prompt":"/code-review"}`
					}
					var stdout, stderr bytes.Buffer
					if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &stdout, &stderr); code != ExitOK || strings.Contains(stderr.String(), "ERROR") {
						t.Fatalf("%s code=%d stderr=%q", event, code, stderr.String())
					}
					return stdout.String()
				}
				first, second := implicit, "UserPromptExpansion"
				if explicitFirst {
					first, second = second, first
				}
				if got := invoke(first); strings.Count(got, "M1B installed skill limit.") != 1 {
					t.Fatalf("first %s output = %q", first, got)
				}
				for _, event := range []string{second, first} {
					if got := invoke(event); strings.TrimSpace(got) != "{}" {
						t.Fatalf("same-session %s repeated skill context: %q", event, got)
					}
				}
				// Recompiling without the skill rule must remove its subscription
				// while retaining the unrelated shell subscription.
				managed := filepath.Join(home, "packages", "skill-install", "STYLE.yaml")
				writeSyncFile(t, managed, strings.Replace(manifest, "target: skill\n    skill: code-review", "target: tool\n    tool: shell", 1))
				if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "skill-install", Yes: true}); err != nil {
					t.Fatal(err)
				}
				updated := readClaudeHookSettings(t, config)
				if len(updated.Hooks["UserPromptExpansion"]) != 0 {
					t.Fatal("explicit skill subscription remained without skill rules")
				}
				if len(updated.Hooks[implicit]) != 1 || updated.Hooks[implicit][0].Matcher != "^(Bash)$" {
					t.Fatalf("unrelated shell subscription changed: %#v", updated.Hooks[implicit])
				}
			})
		}
	}
}
