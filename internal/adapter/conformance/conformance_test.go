// Package conformance_test exercises real adapters through agenthooks' wire
// protocol. Provider-specific interpretation stays in the production adapters.
package conformance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/agenthookstest"
)

const corpus = "../../../testdata/payloads"

type providerSuite struct {
	id       agenthooks.Provider
	facts    adapter.PlanningFacts
	enricher func(*testing.T) adapter.RuntimeEnricher
}

func suites() []providerSuite {
	return []providerSuite{
		{agenthooks.ProviderCodex, codex.DefaultFacts(), func(t *testing.T) adapter.RuntimeEnricher {
			return &codex.Enricher{Home: t.TempDir(), HarnessVersion: "0.159.3",
				LaunchSettings: func(string) (agenthooks.CodexLaunchSettings, bool) {
					return agenthooks.CodexLaunchSettings{IgnoreUserConfig: true}, true
				}, ReadFile: func(string) ([]byte, error) { return nil, os.ErrNotExist }}
		}},
		{agenthooks.ProviderClaudeCode, claude.Facts{StaticAppendAvailable: true}, func(t *testing.T) adapter.RuntimeEnricher {
			return &claude.Enricher{HarnessVersion: "2.1.315", Environment: func() string { return "" }}
		}},
	}
}

func buildIndex(t *testing.T, suite providerSuite, manifest []byte) compile.Index {
	t.Helper()
	pkg, err := spec.LoadSnapshot("/conformance", map[string][]byte{"STYLE.yaml": manifest}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range spec.Validate(pkg) {
		if diagnostic.Severity == "error" {
			t.Fatalf("invalid conformance package: %#v", diagnostic)
		}
	}
	resolved, err := spec.Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := adapter.BuildPlanAdapter(suite.id, suite.facts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := plan.Build(resolved, planner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compile.Build(compile.Request{StyleVersion: "0.3.0", Packages: []compile.PackageInput{{
		Package: resolved, Sequence: 1, Lock: "sha256:" + strings.Repeat("0", 64), Plans: []plan.Report{report},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return result.Index
}

type eventProjection struct {
	Native         string                 `json:"native"`
	Kind           agenthooks.EventKind   `json:"kind"`
	Session        agenthooks.SessionInfo `json:"session"`
	Agent          *agenthooks.AgentInfo  `json:"agent,omitempty"`
	Variables      map[string]string      `json:"variables,omitempty"`
	Points         []pointProjection      `json:"points"`
	PersistedModel string                 `json:"persisted_model,omitempty"`
}

type pointProjection struct {
	Name      string            `json:"name"`
	Canonical string            `json:"canonical,omitempty"`
	Tool      string            `json:"tool,omitempty"`
	MCPServer string            `json:"mcp_server,omitempty"`
	MCPTool   string            `json:"mcp_tool,omitempty"`
	Skill     string            `json:"skill,omitempty"`
	AgentID   string            `json:"agent_id,omitempty"`
	AgentType string            `json:"agent_type,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type observedEnricher struct {
	adapter.RuntimeEnricher
	projection *eventProjection
}

func (observer observedEnricher) Enrich(event *agenthooks.Event, typed any, session state.Session, scope state.Scope) (engine.Input, tokens.InputLimit, error) {
	input, limit, err := observer.RuntimeEnricher.Enrich(event, typed, session, scope)
	observer.projection.Variables = input.Variables
	for _, point := range input.Points {
		fields := map[string]string{}
		for _, key := range []string{"cwd", "prompt", "tool.input", "tool.input.command", "tool.output", "tool.output.stdout", "tool.added", "skill.name", "mcp.server", "mcp.tool"} {
			if value, ok := point.When.Fields.ReadConditionField(key); ok {
				fields[key] = value
			}
		}
		observer.projection.Points = append(observer.projection.Points, pointProjection{
			Name: point.Name, Canonical: point.CanonicalTool, Tool: point.ToolName,
			MCPServer: point.MCPServer, MCPTool: point.MCPTool, Skill: point.SkillName,
			AgentID: point.When.AgentID, AgentType: point.When.AgentType, Fields: fields,
		})
	}
	return input, limit, err
}

func invoke(t *testing.T, suite providerSuite, index compile.Index, payload []byte, store *state.Store) (agenthookstest.Result, eventProjection) {
	t.Helper()
	projection := eventProjection{Points: []pointProjection{}}
	core, err := engine.New(index, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := styleruntime.New(core, store, observedEnricher{suite.enricher(t), &projection}, suite.id, false)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	runtime.Logger = logger
	runner := agenthooks.New(agenthooks.WithoutBackfill(), agenthooks.WithoutDedup(), agenthooks.WithLogger(logger))
	runner.OnAny(func(_ context.Context, event *agenthooks.Event) error {
		projection.Native, projection.Kind, projection.Session, projection.Agent = event.NativeName, event.Kind, event.Session, event.Agent
		return nil
	})
	if err := runtime.Register(runner); err != nil {
		t.Fatal(err)
	}
	result := agenthookstest.Invoke(t, runner, suite.id, payload)
	if result.ExitCode != 0 || result.Stderr != "" || strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("hook failed: %#v logs=%s", result, logs.String())
	}
	session, err := store.ReadSession(projection.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	projection.PersistedModel = session.Model
	return result, projection
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func golden(t *testing.T, path string, actual []byte) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if want := read(t, path); !bytes.Equal(actual, want) {
		t.Fatalf("differs from %s\ngot: %s\nwant: %s", path, actual, want)
	}
}

func newStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestProviderPayloadsThroughRealRuntime(t *testing.T) {
	manifest := read(t, filepath.Join(corpus, "STYLE.yaml"))
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			index := buildIndex(t, suite, manifest)
			paths, err := filepath.Glob(filepath.Join(corpus, string(suite.id), "*.json"))
			if err != nil || len(paths) == 0 {
				t.Fatalf("corpus: %v", err)
			}
			seen := map[string]bool{}
			for _, path := range paths {
				t.Run(filepath.Base(path), func(t *testing.T) {
					payload := read(t, path)
					result, projection := invoke(t, suite, index, payload, newStore(t))
					seen[projection.Native] = true
					checkEvent(t, suite.id, projection)
					golden(t, strings.TrimSuffix(path, ".json")+".stdout.golden", result.Stdout)
					data, err := json.MarshalIndent(projection, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					golden(t, strings.TrimSuffix(path, ".json")+".event.golden", append(data, '\n'))
					emptyIndex := index
					emptyIndex.Harnesses = map[string]map[string][]compile.Entry{string(suite.id): {}}
					noop, _ := invoke(t, suite, emptyIndex, payload, newStore(t))
					agenthookstest.AssertNoOp(t, suite.id, noop)
					checkWhitelist(t, suite.id, result.Stdout)
				})
			}
			for native := range nativeKinds(suite.id) {
				if !seen[native] {
					t.Errorf("missing native event fixture %s", native)
				}
			}
		})
	}
}

func nativeKinds(provider agenthooks.Provider) map[string]agenthooks.EventKind {
	kinds := map[string]agenthooks.EventKind{
		"SessionStart": agenthooks.KindSessionStart, "SessionEnd": agenthooks.KindSessionEnd,
		"UserPromptSubmit": agenthooks.KindPromptSubmitted, "PreToolUse": agenthooks.KindToolPre,
		"PostToolUse": agenthooks.KindToolPost, "PermissionRequest": agenthooks.KindPermission,
		"Stop": agenthooks.KindStop, "SubagentStart": agenthooks.KindSubagentStart,
		"SubagentStop": agenthooks.KindSubagentStop, "PreCompact": agenthooks.KindCompactPre,
		"PostCompact": agenthooks.KindCompactPost, "FutureEvent": agenthooks.KindOther,
	}
	if provider == agenthooks.ProviderClaudeCode {
		for native, kind := range map[string]agenthooks.EventKind{
			"UserPromptExpansion": agenthooks.KindPromptExpansion, "PostToolUseFailure": agenthooks.KindToolError,
			"Notification": agenthooks.KindNotification, "FileChanged": agenthooks.KindFileEdited,
			"MessageDisplay": agenthooks.KindModelResponse, "PostModelSwitch": agenthooks.KindOther, "Setup": agenthooks.KindOther,
		} {
			kinds[native] = kind
		}
	}
	return kinds
}

func checkEvent(t *testing.T, provider agenthooks.Provider, event eventProjection) {
	t.Helper()
	if want, ok := nativeKinds(provider)[event.Native]; !ok || event.Kind != want {
		t.Fatalf("unexpected event %s/%s", event.Native, event.Kind)
	}
	point := map[agenthooks.EventKind]string{agenthooks.KindSessionStart: "session.start", agenthooks.KindPromptSubmitted: "prompt.submitted",
		agenthooks.KindPromptExpansion: "skill.explicit", agenthooks.KindToolPre: "tool.pre", agenthooks.KindToolPost: "tool.post",
		agenthooks.KindToolError: "tool.error", agenthooks.KindPermission: "tool.pre", agenthooks.KindSubagentStart: "subagent.start"}[event.Kind]
	if point == "" && len(event.Points) != 0 || point != "" && (len(event.Points) == 0 || event.Points[0].Name != point) {
		t.Fatalf("%s eval points = %#v, want first %q", event.Native, event.Points, point)
	}
}
