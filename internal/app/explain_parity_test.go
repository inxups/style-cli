package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/speakeasy-api/agenthooks"
)

func TestM3A1SnapshotLifecycleMatchesOrdinaryRuntime(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, mode := range []string{"startup", "clear", "compact", "resume", "fork", "turn-native", "turn-fallback", "turn-new", "every", "changed-body", "child", "prompt", "permission", "tool-error", "explicit-skill", "model-switch"} {
			if harness == "codex" && (mode == "tool-error" || mode == "explicit-skill" || mode == "model-switch") {
				continue
			}
			t.Run(harness+"/"+mode, func(t *testing.T) {
				event, point, source, agentID := "PreToolUse", "tool.pre", "", "main"
				payload := map[string]any{"session_id": "s", "tool_name": "Bash", "tool_input": map[string]string{"command": "harmless"}}
				switch mode {
				case "startup", "clear", "compact", "resume", "fork":
					event, point, source = "SessionStart", "session.start", mode
					payload["source"] = source
				case "child":
					agentID = "child-1"
					payload["agent_id"], payload["agent_type"] = agentID, "reviewer"
				case "prompt":
					event, point = "UserPromptSubmit", "prompt.submitted"
					payload["prompt"] = "captured prompt"
				case "permission":
					event = "PermissionRequest"
				case "tool-error":
					event, point = "PostToolUseFailure", "tool.error"
					payload["error"] = "captured failure"
				case "explicit-skill":
					event, point = "UserPromptExpansion", "skill.explicit"
					payload["expansion_type"], payload["command_name"] = "slash_command", "review"
				case "model-switch":
					event, point = "PostModelSwitch", ""
					payload["to_model"] = "new-model"
				}
				entry := a1Entry("replay", point, "Replay body.")
				entry.When.Agent = []string{"*"}
				if mode == "permission" {
					entry.Action = "block"
				}
				if mode == "explicit-skill" {
					entry.Target, entry.Action = "skill", "block"
					entry.Match = &compile.Match{Skill: []string{"review"}}
				}
				entry.ReinjectOnCompact = true
				if point == "session.start" {
					sources := []string{"startup", "clear", "compact", "resume", "fork"}
					entry.Sources = &sources
				}
				turnID := ""
				if mode == "turn-native" || mode == "turn-new" {
					turnID = "known-turn"
					if mode == "turn-new" {
						turnID = "new-turn"
					}
					payload["turn_id"], payload["prompt_id"] = turnID, turnID
				}
				if mode == "turn-native" || mode == "turn-fallback" || mode == "turn-new" {
					entry.Repeat = "turn"
				} else if mode == "every" {
					entry.Repeat = "every"
				}
				entries := map[string][]compile.Entry{}
				if point != "" {
					entries[point] = []compile.Entry{entry}
				}
				service, request := a1Fixture(t, harness, event, entries)
				payload["hook_event_name"] = event
				encoded, _ := json.Marshal(payload)
				writeSyncFile(t, request.Payload, string(encoded))
				request.StateDir = filepath.Join(t.TempDir(), "source")
				seed, _ := state.New(request.StateDir)
				_, scope, err := seed.Update("s", agentID, state.EventUpdate{Source: "clear", PromptSubmitted: true, Model: "old-model"})
				if err != nil {
					t.Fatal(err)
				}
				_, scope, err = seed.Update("s", agentID, state.EventUpdate{Source: "compact", PromptSubmitted: true})
				if err != nil {
					t.Fatal(err)
				}
				seedBody, seedTurn := "Replay body.", turnID
				if mode == "changed-body" {
					seedBody = "Old body."
				}
				if mode == "turn-new" {
					seedTurn = "known-turn"
				}
				if _, _, err := seed.Claim(state.ClaimInput{Scope: scope, Rule: entry.Rule, Action: entry.Action, Event: point, Repeat: entry.Repeat, Body: seedBody, TurnID: seedTurn, ReinjectOnCompact: true}); err != nil {
					t.Fatal(err)
				}
				before := snapshotStatsTree(t, request.StateDir)
				result, err := service.Explain(context.Background(), request)
				if err != nil || result.Wire == nil {
					t.Fatalf("replay %+v %v", result, err)
				}
				if !reflect.DeepEqual(before, snapshotStatsTree(t, request.StateDir)) {
					t.Fatalf("replay changed the source snapshot: before=%+v after=%+v", before, snapshotStatsTree(t, request.StateDir))
				}
				ordinaryRoot := filepath.Join(t.TempDir(), "ordinary")
				if _, err := state.CopySnapshot(context.Background(), request.StateDir, ordinaryRoot, "s", agentID); err != nil {
					t.Fatal(err)
				}
				store, _ := state.New(ordinaryRoot)
				core, err := engine.LoadIndexBytes(readSyncFile(t, request.Index), store)
				if err != nil {
					t.Fatal(err)
				}
				var enricher adapter.RuntimeEnricher
				if harness == "codex" {
					value := codex.NewEnricher()
					value.Home = t.TempDir()
					value.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) { return agenthooks.CodexLaunchSettings{}, false }
					value.ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
					enricher = value
				} else {
					value := claude.NewEnricher()
					value.Environment = func() string { return "" }
					enricher = value
				}
				runtime, err := styleruntime.New(core, store, enricher, agenthooks.Provider(harness), false)
				if err != nil {
					t.Fatal(err)
				}
				runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
				runtime.CodecStateDir = t.TempDir()
				var stdout, stderr bytes.Buffer
				exit := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + harness}, bytes.NewReader(encoded), &stdout, &stderr, agenthooks.WithoutMCPResolution())
				if result.Wire.Stdout != stdout.String() || result.Wire.Stderr != stderr.String() || result.Wire.ExitCode != exit {
					t.Fatalf("ordinary wire differs: %+v %q %q %d", result.Wire, stdout.String(), stderr.String(), exit)
				}
				_, after, err := store.Read("s", agentID)
				if err != nil || explainCounters(after) != result.State.After {
					t.Fatalf("ordinary scope differs: %+v %+v %v", after, result.State, err)
				}
				if point != "" && len(result.Entries) != 1 {
					t.Fatal("compiled entry was not traced")
				}
				if mode == "turn-native" && result.Entries[0].TurnSource != "native" || mode == "turn-fallback" && result.Entries[0].TurnSource != "prompt-count" {
					t.Fatal("turn source was guessed from its value")
				}
			})
		}
	}
}
