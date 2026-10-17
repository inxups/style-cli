package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/speakeasy-api/agenthooks"
)

func TestRefreshRuntimeClockOnlyAndNativeWire(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, provider := range []agenthooks.Provider{agenthooks.ProviderCodex, agenthooks.ProviderClaudeCode} {
		t.Run(string(provider), func(t *testing.T) {
			root := t.TempDir()
			store, _ := state.New(root)
			entry := runtimeTestEntry("pkg/r", "tool.pre", "event", "inject", staticBody("periodic context"))
			entry.Refresh = 3
			index := runtimeTestIndex(map[string][]compile.Entry{"tool.pre": {entry}})
			index.Format = 2
			core, err := engine.New(index, store)
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := New(core, store, testEnricher{}, provider, false)
			if err != nil {
				t.Fatal(err)
			}
			trace := engine.Trace{}
			runtime.Trace = &trace
			for _, tc := range []struct {
				event, id string
				want      bool
				source    string
			}{
				{"SessionStart", "", false, "startup"}, {"PreToolUse", "", true, ""},
				{"UserPromptSubmit", "secret-a", false, ""}, {"UserPromptSubmit", "secret-a", false, ""},
				{"UserPromptExpansion", "secret-a", false, ""}, {"PostModelSwitch", "secret-a", false, ""},
				{"UserPromptSubmit", "secret-b", false, ""}, {"PreToolUse", "secret-b", false, ""},
				{"UserPromptSubmit", "secret-c", false, ""}, {"PreToolUse", "secret-a", false, ""},
				{"PreToolUse", "secret-c", true, ""}, {"PreToolUse", "secret-c", false, ""},
			} {
				payload := map[string]any{"session_id": "s", "hook_event_name": tc.event, "source": tc.source, "tool_name": "Bash", "tool_input": map[string]string{"command": "PRIVATE_COMMAND"}, "prompt": "PRIVATE_PROMPT", "model": "model-a", "to_model": "model-b", "expansion_type": "slash_command", "command_name": "review"}
				key := "turn_id"
				if provider == agenthooks.ProviderClaudeCode {
					key = "prompt_id"
				}
				payload[key] = tc.id
				data, _ := json.Marshal(payload)
				var out, logs bytes.Buffer
				args := []string{"agenthooks", "run", "--provider=" + string(provider)}
				code := runtime.Run(context.Background(), args, bytes.NewReader(data), &out, &logs)
				if code != 0 || strings.Contains(logs.String(), "level=ERROR") || (strings.Contains(out.String(), "periodic context") != tc.want) {
					t.Fatalf("%+v exit=%d wire=%s logs=%s trace=%+v", tc, code, out.String(), logs.String(), trace)
				}
				if tc.event == "PreToolUse" {
					if len(trace.Entries) != 1 || trace.Entries[0].Refresh == nil {
						t.Fatal(trace)
					}
					safe, _ := json.Marshal(trace)
					if bytes.Contains(safe, []byte("secret-")) || bytes.Contains(safe, []byte("PRIVATE")) {
						t.Fatal(string(safe))
					}
				}
			}
			data, err := os.ReadFile(filepath.Join(root, "s", "main", "refresh", string(provider)+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var clock struct {
				Q int `json:"q"`
			}
			json.Unmarshal(data, &clock)
			if clock.Q != 3 {
				t.Fatal(string(data))
			}
			stats, err := os.ReadFile(filepath.Join(root, "s", "main", "stats.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(stats), "secret-") || strings.Contains(string(stats), "PRIVATE") {
				t.Fatal(string(stats))
			}
		})
	}
}

func TestRefreshDisabledHasNoClockOrGuardAndActiveHarnessOnly(t *testing.T) {
	root := t.TempDir()
	store, _ := state.New(root)
	entry := runtimeTestEntry("pkg/r", "tool.pre", "event", "inject", staticBody("old context"))
	index := runtimeTestIndex(map[string][]compile.Entry{"tool.pre": {entry}})
	active := entry
	active.Refresh = 3
	index.Format = 2
	index.Harnesses["codex"] = map[string][]compile.Entry{"tool.pre": {active}}
	core, err := engine.New(index, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, _ := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, false)
	var out, logs bytes.Buffer
	if code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(claudeToolPrePayload), &out, &logs); code != 0 || strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(out.String(), "old context") {
		t.Fatal(code, out.String(), logs.String())
	}
	for _, path := range []string{"sess-claude-1/refresh.lock", "sess-claude-1/main/refresh"} {
		if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatal("inactive harness created refresh state", path, err)
		}
	}
}

func TestRefreshBusyUsesExistingFailPolicy(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprint(closed), func(t *testing.T) {
			store, _ := state.New(t.TempDir())
			entry := runtimeTestEntry("pkg/r", "tool.pre", "event", "inject", staticBody("context"))
			entry.Refresh = 1
			index := runtimeTestIndex(map[string][]compile.Entry{"tool.pre": {entry}})
			index.Format = 2
			core, err := engine.New(index, store)
			if err != nil {
				t.Fatal(err)
			}
			runtime, _ := New(core, store, testEnricher{}, agenthooks.ProviderClaudeCode, closed)
			guard, err := store.BeginRefresh(context.Background(), "sess-claude-1")
			if err != nil {
				t.Fatal(err)
			}
			defer guard.Close()
			var out, logs bytes.Buffer
			code := runtime.Run(context.Background(), claudeRunArgs(), strings.NewReader(claudeToolPrePayload), &out, &logs)
			if strings.Contains(out.String(), "context") || !strings.Contains(logs.String(), "state-error") {
				t.Fatal(code, out.String(), logs.String())
			}
			if closed && !strings.Contains(out.String(), `"deny"`) {
				t.Fatal("fail-closed did not deny", code, out.String())
			}
		})
	}
}
