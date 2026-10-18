package conformance_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/template"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

func TestM3A10RealCodecsFencedEventFields(t *testing.T) {
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			added := map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch", "tool_input": map[string]any{"command": "*** Begin Patch\n*** Add File: fixture\n+ADDED_CANARY\n*** End Patch"}}
			skill := map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "$review"}
			if string(suite.id) == "claude-code" {
				added = map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write", "tool_input": map[string]any{"file_path": "/fixture/a", "content": "ADDED_CANARY"}}
				skill = map[string]any{"hook_event_name": "UserPromptExpansion", "expansion_type": "slash_command", "command_name": "review", "prompt": "/review"}
			}
			cases := []struct {
				event, field string
				payload      map[string]any
				value        string
			}{
				{"prompt.submitted", "prompt", map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "PROMPT_CANARY\n```</style-untrusted-event>"}, "PROMPT_CANARY\n```</style-untrusted-event>"},
				{"tool.pre", "tool.input.command", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "COMMAND_CANARY\n`<>&", "nested": map[string]any{"value": "nested"}}}, "COMMAND_CANARY\n`<>&"},
				{"tool.pre", "tool.input.nested.value", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"nested": map[string]any{"value": "nested"}}}, "nested"},
				{"tool.post", "tool.output", map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "true"}, "tool_response": "OUTPUT_CANARY\u202e"}, "OUTPUT_CANARY\u202e"},
				{"tool.pre", "tool.input", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "true"}}, `{"command":"true"}`},
				{"tool.pre", "tool.added", added, "ADDED_CANARY"},
				{"tool.pre", "mcp.server", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "mcp__fixture__echo", "tool_input": map[string]any{"text": "x"}}, "fixture"},
				{"tool.pre", "mcp.tool", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "mcp__fixture__echo", "tool_input": map[string]any{"text": "x"}}, "echo"},
				{"skill.explicit", "skill.name", skill, "review"},
				{"tool.pre", "tool.name", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "true"}}, "Bash"},
			}
			for _, tc := range cases {
				t.Run(tc.field, func(t *testing.T) {
					manifest := fmt.Sprintf("spec: style/v0.3\nname: event-content\nversion: 1.0.0\ndescription: content fixture\nrules:\n  - {id: r, target: event, event: %s, text: '{{event.%s}}'}\n", tc.event, tc.field)
					if tc.event == "skill.explicit" {
						manifest = strings.Replace(manifest, "target: event, event: skill.explicit", "target: skill, skill: review, at: before", 1)
					}
					index := buildIndex(t, suite, []byte(manifest))
					if index.Format != 3 {
						t.Fatal(index.Format)
					}
					tc.payload["session_id"] = "event-fixture"
					tc.payload["cwd"] = "/fixture"
					raw, _ := json.Marshal(tc.payload)
					result, _ := invoke(t, suite, index, raw, newStore(t))
					var wire struct {
						Hook struct {
							Context string `json:"additionalContext"`
						} `json:"hookSpecificOutput"`
					}
					if err := json.Unmarshal(result.Stdout, &wire); err != nil {
						t.Fatal(err, string(result.Stdout))
					}
					expected := template.FenceEvent("event."+tc.field, tc.value)
					if wire.Hook.Context != expected {
						t.Fatalf("%s got %q want %q", suite.id, wire.Hook.Context, expected)
					}
					checkWhitelist(t, suite.id, result.Stdout)
					count := tokenestimate.CountText(expected)
					entry := index.Harnesses[string(suite.id)][tc.event][0]
					if count.Tokens > entry.Estimate.Tokens || count.Chars > entry.Estimate.Chars {
						t.Fatal("underestimated fenced event")
					}
					if !strings.Contains(entry.Needs[0]+strings.Join(entry.Needs, ","), "event.") {
						t.Fatal(entry.Needs)
					}
				})
			}
		})
	}
}
