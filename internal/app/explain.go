package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/inxups/style-cli/internal/tokencount"
	"github.com/speakeasy-api/agenthooks"
)

type ExplainRequest struct {
	Count                                                       tokencount.Options
	Harness, Event, Payload, Index, StateDir, Facts, Provenance string
	FailClosed                                                  bool
}
type ExplainDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
type ExplainInputs struct {
	EffortMappingSHA256 string `json:"effort_mapping_sha256,omitempty"`
	IndexSHA256         string `json:"index_sha256,omitempty"`
	PayloadSHA256       string `json:"payload_sha256,omitempty"`
	FactsSHA256         string `json:"facts_sha256,omitempty"`
	FailClosed          bool   `json:"fail_closed"`
}
type ExplainCounters struct {
	Generation  uint64 `json:"gen"`
	Epoch       uint64 `json:"epoch"`
	PromptCount uint64 `json:"prompt_count"`
}
type ExplainState struct {
	Mode            string          `json:"mode"`
	Quality         string          `json:"quality,omitempty"`
	SeedScopeExists bool            `json:"seed_scope_exists"`
	ManifestSHA256  string          `json:"manifest_sha256,omitempty"`
	Before          ExplainCounters `json:"before"`
	After           ExplainCounters `json:"after"`
}
type ExplainFact struct {
	Field     string `json:"field"`
	Available bool   `json:"available"`
	Source    string `json:"source"`
}
type ExplainWire struct {
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ExitCode     int    `json:"exit_code"`
	StdoutBytes  int    `json:"stdout_bytes"`
	StderrBytes  int    `json:"stderr_bytes"`
	StdoutSHA256 string `json:"stdout_sha256"`
	StderrSHA256 string `json:"stderr_sha256"`
	Stage        string `json:"stage"`
	FailPolicy   string `json:"fail_policy"`
}
type ExplainStatic struct {
	compile.StaticEntry
	Status string `json:"status"`
}
type ExplainProvenance struct {
	Status   string                         `json:"status"`
	Coverage string                         `json:"coverage"`
	SHA256   string                         `json:"sha256,omitempty"`
	Rules    []compile.ProvenanceRule       `json:"rules"`
	Static   []ExplainStatic                `json:"static"`
	Native   []compile.ProvenanceNativeHook `json:"native"`
}
type ExplainResult struct {
	Count             *tokencount.Result   `json:"count,omitempty"`
	Format            int                  `json:"format"`
	Command           string               `json:"command"`
	Status            string               `json:"status"`
	Harness           string               `json:"harness"`
	Event             string               `json:"event"`
	Inputs            ExplainInputs        `json:"inputs"`
	State             ExplainState         `json:"state"`
	Facts             []ExplainFact        `json:"facts"`
	Points            []engine.PointTrace  `json:"points"`
	Entries           []*engine.EntryTrace `json:"entries"`
	CompileProvenance ExplainProvenance    `json:"compile_provenance"`
	Wire              *ExplainWire         `json:"wire"`
	Diagnostics       []ExplainDiagnostic  `json:"diagnostics"`
}
type explainFacts struct {
	ModelProvider     *string `json:"model_provider,omitempty"`
	Format            int     `json:"format"`
	Harness           string  `json:"harness"`
	PayloadSHA256     string  `json:"payload_sha256"`
	HarnessVersion    *string `json:"harness_version,omitempty"`
	CodexConfigEffort *string `json:"codex_config_effort,omitempty"`
	ClaudeEnvEffort   *string `json:"claude_env_effort,omitempty"`
	MCP               *struct {
		Server string `json:"server"`
		Tool   string `json:"tool"`
	} `json:"mcp,omitempty"`
	ClaudeSkillContent *string `json:"claude_skill_content,omitempty"`
}
type explainOps struct {
	mktemp   func(string, string) (string, error)
	remove   func(string) error
	snapshot func(context.Context, string, string, string, string) (state.Snapshot, error)
}

func defaultExplainOps() explainOps {
	return explainOps{os.MkdirTemp, os.RemoveAll, state.CopySnapshot}
}

// Explain captures explicit inputs and replays a native event in a private root.
// No detector, installed route, package source or actual hook entry is called.
func (s *Service) Explain(ctx context.Context, request ExplainRequest) (ExplainResult, error) {
	return s.explain(ctx, request, defaultExplainOps())
}

func (s *Service) explain(ctx context.Context, request ExplainRequest, ops explainOps) (result ExplainResult, returned error) {
	result = ExplainResult{Format: 1, Command: "explain", Status: "error", Harness: request.Harness, Event: request.Event, State: ExplainState{Mode: "empty"}, Facts: []ExplainFact{}, Points: []engine.PointTrace{}, Entries: []*engine.EntryTrace{}, CompileProvenance: ExplainProvenance{Status: "unavailable", Coverage: "index-only", Rules: []compile.ProvenanceRule{}, Static: []ExplainStatic{}, Native: []compile.ProvenanceNativeHook{}}, Diagnostics: []ExplainDiagnostic{}}
	result.Inputs.FailClosed = request.FailClosed
	fail := func(code, message string) error {
		result.Status = "error"
		result.Diagnostics = append(result.Diagnostics, ExplainDiagnostic{code, "error", message})
		return errors.New(code)
	}
	if err := request.Count.Validate(); err != nil {
		return result, fail("count-options-invalid", err.Error())
	}
	if err := checkContext(ctx); err != nil {
		return result, fail("cancelled", "replay was cancelled")
	}
	if name := string(harnessName(request.Harness)); name != request.Harness {
		request.Harness = name
		result.Harness = name
	}
	if _, ok := s.definitions[adapter.ProviderID(request.Harness)]; !ok {
		return result, fail("harness-unknown", "harness is not registered")
	}
	provider := agenthooks.Provider(request.Harness)
	if provider != agenthooks.ProviderClaudeCode && provider != agenthooks.ProviderCodex {
		return result, fail("runtime-unavailable", "registered harness has no runtime replay adapter")
	}
	if request.FailClosed && provider != agenthooks.ProviderClaudeCode {
		return result, fail("fail-closed-unsupported", "installation fail-closed is available only for Claude Code")
	}
	supported := func(event string) bool {
		return agenthooks.RecognizesNativeEvent(provider, event) || provider == agenthooks.ProviderClaudeCode && event == "PostModelSwitch"
	}
	if request.Event != "" && !supported(request.Event) {
		return result, fail("event-unsupported", "native event is not supported by the selected runtime")
	}
	payload, err := saferead.ReadFile(request.Payload, 32<<20)
	if err != nil {
		return result, fail("payload-invalid", "payload must be a bounded regular UTF-8 JSON file")
	}
	result.Inputs.PayloadSHA256 = explainSHA(payload)
	var raw map[string]json.RawMessage
	if !utf8.Valid(payload) || json.Unmarshal(payload, &raw) != nil || raw == nil {
		return result, fail("payload-invalid", "payload must be one UTF-8 JSON object")
	}
	var native, sessionID, agentID, toolName string
	if json.Unmarshal(raw["hook_event_name"], &native) != nil || native == "" || json.Unmarshal(raw["session_id"], &sessionID) != nil || sessionID == "" {
		return result, fail("payload-invalid", "native event and session ID must be nonempty strings")
	}
	if request.Event == "" {
		// The payload is authoritative; an explicit --event only cross-checks it.
		request.Event, result.Event = native, native
		if !supported(native) {
			return result, fail("event-unsupported", "native event is not supported by the selected runtime")
		}
	}
	if native != request.Event {
		return result, fail("event-mismatch", "native event differs from the payload event")
	}
	_ = json.Unmarshal(raw["agent_id"], &agentID)
	_ = json.Unmarshal(raw["tool_name"], &toolName)
	facts := explainFacts{}
	if request.Facts != "" {
		data, e := saferead.ReadFile(request.Facts, 1<<20)
		if e != nil || !utf8.Valid(data) || decodeExplainJSON(data, &facts) != nil {
			return result, fail("facts-invalid", "supplementary facts have an invalid format or file type")
		}
		result.Inputs.FactsSHA256 = explainSHA(data)
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil {
			return result, fail("facts-invalid", "facts must be one JSON object")
		}
		for _, value := range fields {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return result, fail("facts-invalid", "supplementary facts cannot contain null fields")
			}
		}
		if facts.Format != 1 || facts.Harness != request.Harness || facts.PayloadSHA256 != result.Inputs.PayloadSHA256 {
			return result, fail("facts-conflict", "supplementary facts do not match this harness and exact payload")
		}
		if provider != agenthooks.ProviderCodex && facts.CodexConfigEffort != nil || provider != agenthooks.ProviderClaudeCode && (facts.ClaudeEnvEffort != nil || facts.ClaudeSkillContent != nil) {
			return result, fail("facts-conflict", "supplementary facts belong to another harness")
		}
		if facts.ModelProvider != nil && *facts.ModelProvider != "" && !when.ValidProviderID(*facts.ModelProvider) {
			return result, fail("facts-invalid", "provider fact must be a bounded service identifier or empty")
		}
		if facts.MCP != nil && (facts.MCP.Server == "" || facts.MCP.Tool == "") {
			return result, fail("facts-invalid", "MCP facts require server and tool identities")
		}
		if facts.ClaudeSkillContent != nil && (request.Event != "PostToolUse" || !strings.EqualFold(toolName, "Skill") || len(*facts.ClaudeSkillContent) > 65536) {
			return result, fail("facts-conflict", "skill content requires a successful Claude Skill PostToolUse event")
		}
	}
	if request.Index == "" {
		home, e := s.resolveStyleHome("")
		if e != nil {
			return result, fail("index-invalid", "cannot locate the default index")
		}
		if s.project != nil {
			if _, e = s.loadLock(home); e != nil {
				return result, fail("project-binding-invalid", e.Error())
			}
		}
		request.Index = filepath.Join(home, "build", "index.json")
	}
	indexBytes, err := saferead.ReadFile(request.Index, 16<<20)
	if err != nil {
		return result, fail("index-invalid", "index must be a bounded regular file")
	}
	result.Inputs.IndexSHA256 = explainSHA(indexBytes)
	private, err := ops.mktemp("", "style-explain-*")
	if err != nil {
		return result, fail("temporary-state-error", "cannot create private replay storage")
	}
	defer func() {
		if err := ops.remove(private); err != nil {
			returned = fail("cleanup-failed", "cannot remove private replay storage: "+private)
		}
	}()
	if err := os.Chmod(private, 0700); err != nil {
		return result, fail("temporary-state-error", "cannot secure private replay storage")
	}
	storeRoot := filepath.Join(private, "state")
	if request.StateDir != "" {
		snap, e := ops.snapshot(ctx, request.StateDir, storeRoot, sessionID, agentID)
		if e != nil {
			code := "state-snapshot-invalid"
			if errors.Is(e, saferead.ErrChanged) {
				code = "state-snapshot-changed"
			}
			if ctx.Err() != nil {
				code = "cancelled"
			}
			return result, fail(code, "cannot capture the selected state without changes or invalid input")
		}
		result.State.Mode = "snapshot"
		result.State.Quality = "captured-nonatomic"
		result.State.SeedScopeExists = snap.ScopeExists
		result.State.ManifestSHA256 = snap.ManifestSHA256
	}
	store, err := state.New(storeRoot)
	if err != nil {
		return result, fail("temporary-state-error", "cannot initialize private replay state")
	}
	previous, before, err := store.Read(sessionID, agentID)
	if err != nil {
		return result, fail("state-snapshot-invalid", "selected state cannot be decoded")
	}
	result.State.Before = explainCounters(before)
	result.State.After = result.State.Before
	core, err := engine.LoadIndexBytes(indexBytes, store)
	if err != nil {
		return result, fail("index-invalid", "index format, schema or compiled conditions are invalid")
	}
	var index compile.Index
	if json.Unmarshal(indexBytes, &index) != nil {
		return result, fail("index-invalid", "cannot decode the captured index")
	}
	s.explainProvenance(&result, request, index, indexBytes)
	// One bounded recapture tolerates the index/sidecar transaction window.
	if result.CompileProvenance.Status == "stale" {
		if recaptured, e := saferead.ReadFile(request.Index, 16<<20); e == nil && !bytes.Equal(recaptured, indexBytes) {
			if candidate, e := engine.LoadIndexBytes(recaptured, store); e == nil {
				core = candidate
				indexBytes = recaptured
				_ = json.Unmarshal(indexBytes, &index)
				result.Inputs.IndexSHA256 = explainSHA(indexBytes)
				result.Diagnostics = []ExplainDiagnostic{}
				s.explainProvenance(&result, request, index, indexBytes)
			}
		}
	}
	result.Inputs.EffortMappingSHA256 = index.EffortMapping.SHA256(string(provider))
	var enricher adapter.RuntimeEnricher
	if provider == agenthooks.ProviderCodex {
		value := codex.NewEnricher()
		value.Offline = true
		value.ProviderOffline = true
		value.OfflineProvider = facts.ModelProvider
		value.OfflineConfigEffort = facts.CodexConfigEffort
		if facts.HarnessVersion != nil {
			value.HarnessVersion = *facts.HarnessVersion
		}
		enricher = value
	} else {
		value := claude.NewEnricher()
		value.ProviderOffline = true
		value.OfflineProvider = facts.ModelProvider
		value.Environment = func() string {
			if facts.ClaudeEnvEffort != nil {
				return *facts.ClaudeEnvEffort
			}
			return ""
		}
		if facts.HarnessVersion != nil {
			value.HarnessVersion = *facts.HarnessVersion
		}
		enricher = value
	}
	runtime, err := styleruntime.New(core, store, enricher, provider, request.FailClosed)
	if err != nil {
		return result, fail("runtime-unavailable", "cannot initialize replay runtime")
	}
	trace := engine.Trace{Points: []engine.PointTrace{}, Entries: []*engine.EntryTrace{}}
	runtime.Trace = &trace
	var rendered engine.Output
	if request.Count.Exact() {
		runtime.CapturedOutput = &rendered
	}
	runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	runtime.CodecStateDir = filepath.Join(private, "cache")
	offline := agenthooks.OfflineInputs{SkillContent: facts.ClaudeSkillContent}
	if facts.MCP != nil {
		offline.MCP = &agenthooks.MCPIdentity{Server: facts.MCP.Server, Tool: facts.MCP.Tool}
	}
	var stdout, stderr bytes.Buffer
	outcome := agenthooks.RunOutcome{}
	exit := runtime.Run(ctx, []string{"agenthooks", "run", "--provider=" + request.Harness}, bytes.NewReader(payload), &stdout, &stderr, agenthooks.WithOfflineInputs(offline), agenthooks.WithRunObserver(func(value agenthooks.RunOutcome) { outcome = value }))
	result.Points = trace.Points
	result.Entries = trace.Entries
	if outcome.Encoded {
		result.Wire = &ExplainWire{stdout.String(), stderr.String(), exit, stdout.Len(), stderr.Len(), explainSHA(stdout.Bytes()), explainSHA(stderr.Bytes()), outcome.Stage, outcome.FailPolicy}
	}
	afterSession, after, e := store.Read(sessionID, agentID)
	if e != nil {
		return result, fail("state-error", "cannot read replay results")
	}
	result.State.After = explainCounters(after)
	result.Facts = explainFactMetadata(raw, facts, previous, afterSession, request.Harness)
	for _, entry := range result.Entries {
		for i := range entry.Conditions {
			check := &entry.Conditions[i]
			if !check.Available {
				check.Source = "unavailable"
				continue
			}
			check.Source = "payload"
			for _, fact := range result.Facts {
				if check.Field == fact.Field {
					check.Source = fact.Source
				}
			}
			if check.Field == "harness.name" {
				check.Source = "provided"
			}
			if check.Field == "agent" {
				if agentID == "" {
					check.Source = "session"
				}
			}
			if strings.Contains(check.Field, ".tool.output") && facts.ClaudeSkillContent != nil {
				check.Source = "provided"
			}
			if strings.Contains(check.Field, ".mcp.") && facts.MCP != nil {
				check.Source = "provided"
			}
		}
	}
	if ctx.Err() != nil {
		return result, fail("cancelled", "replay was cancelled")
	}
	if outcome.ErrorCode != "" {
		return result, fail(outcome.ErrorCode, "native replay failed during "+outcome.Stage)
	}
	if !outcome.Encoded {
		return result, fail("runtime-error", "replay did not reach native output encoding")
	}
	if len(trace.Points) == 0 && native != "PostModelSwitch" {
		result.Diagnostics = append(result.Diagnostics, ExplainDiagnostic{"no-style-handler", "info", "recognized native event has no STYLE evaluation handler"})
	}
	if request.Count.Exact() {
		text := modelText(rendered)
		var counted tokencount.Result
		var err error
		if text == "" {
			counted, err = tokencount.None(request.Count)
		} else {
			counted, err = s.countText(ctx, text, request.Count)
		}
		if err != nil {
			return result, fail("count-failed", err.Error())
		}
		result.Count = &counted
	}
	result.Status = "ok"
	return result, nil
}

func (s *Service) explainProvenance(result *ExplainResult, request ExplainRequest, index compile.Index, indexBytes []byte) {
	p := ExplainProvenance{Status: "unavailable", Coverage: "index-only", Rules: []compile.ProvenanceRule{}, Static: []ExplainStatic{}, Native: []compile.ProvenanceNativeHook{}}
	for _, item := range index.Static[request.Harness] {
		p.Static = append(p.Static, ExplainStatic{item, "static-not-replayed"})
	}
	path := request.Provenance
	if path == "" {
		path = filepath.Join(filepath.Dir(request.Index), "explain.json")
	}
	data, err := saferead.ReadFile(path, 16<<20)
	if err == nil {
		p.SHA256 = explainSHA(data)
		provenance, e := compile.DecodeProvenance(data, indexBytes, index)
		if e != nil {
			p.Status = "invalid"
			var binding struct {
				IndexSHA256 string `json:"index_sha256"`
			}
			if json.Unmarshal(data, &binding) == nil && binding.IndexSHA256 != "" && binding.IndexSHA256 != explainSHA(indexBytes) {
				p.Status = "stale"
			}
		} else {
			p.Status = "matched"
			p.Coverage = "compiled-plan"
			p.Rules = append(p.Rules, provenance.Harnesses[request.Harness]...)
			for _, hook := range provenance.NativeHooks[request.Harness] {
				if hook.Native != nil {
					hook.Status = "native-not-executed"
				}
				p.Native = append(p.Native, hook)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		p.Status = "invalid"
	}
	if p.Status != "matched" {
		result.Diagnostics = append(result.Diagnostics, ExplainDiagnostic{"provenance-" + p.Status, "warning", "Compilation provenance is " + p.Status + "; index-only coverage."})
	}
	result.CompileProvenance = p
}

func explainSHA(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
func explainCounters(scope state.Scope) ExplainCounters {
	return ExplainCounters{scope.Generation, scope.Epoch, scope.PromptCount}
}
func decodeExplainJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func explainFactMetadata(raw map[string]json.RawMessage, facts explainFacts, before, after state.Session, harness string) []ExplainFact {
	result := []ExplainFact{}
	add := func(field string, available bool, source string) {
		if !available {
			source = "unavailable"
		}
		result = append(result, ExplainFact{field, available, source})
	}
	var model, effort string
	_ = json.Unmarshal(raw["model"], &model)
	var native string
	_ = json.Unmarshal(raw["hook_event_name"], &native)
	if native == "PostModelSwitch" {
		var switched string
		_ = json.Unmarshal(raw["to_model"], &switched)
		if switched != "" {
			model = switched
		}
	}
	source := "session"
	if model != "" {
		source = "payload"
	}
	add("model", after.Model != "", source)
	if after.ProviderResolved {
		add("provider", after.ModelProvider != "", after.ProviderSource)
	}
	source = "session"
	if facts.HarnessVersion != nil && *facts.HarnessVersion != "" {
		source = "provided"
	}
	add("harness.version", after.HarnessVersion != "", source)
	var e struct {
		Level string `json:"level"`
	}
	_ = json.Unmarshal(raw["effort"], &e)
	effort = e.Level
	source = "session"
	if harness == "codex" {
		if !before.EffortResolved && facts.CodexConfigEffort != nil {
			source = "provided"
		}
	} else if effort != "" {
		source = "payload"
	} else if facts.ClaudeEnvEffort != nil && *facts.ClaudeEnvEffort != "" {
		source = "provided"
	}
	rawSource := source
	if after.EffortMappingSHA256 != "" {
		source = "mapping:" + after.EffortMappingSource
	}
	add("thinking_effort", after.ThinkingEffort != "", source)
	add("thinking_effort_raw", after.EffortRaw != "", rawSource)
	add("thinking_effort_approx", after.EffortApprox != nil, source)
	var toolName string
	_ = json.Unmarshal(raw["tool_name"], &toolName)
	var server struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	_ = json.Unmarshal(raw["mcp_server"], &server)
	mcpAvailable := agenthooks.ParseMCPName(toolName) != nil || server.Name != "" || server.Source != ""
	source = "payload"
	if facts.MCP != nil {
		mcpAvailable = true
		source = "provided"
	}
	add("mcp", mcpAvailable, source)
	add("claude_skill_content", facts.ClaudeSkillContent != nil, "provided")
	for _, field := range []string{"configuration-files", "process-ancestry", "environment", "transcript-backfill", "mcp-discovery", "skill-manifest-files", "network"} {
		add(field, false, "unavailable")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Field < result[j].Field })
	return result
}
