package engine

import (
	"bytes"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
)

const testSHA = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func TestLoadIndexChecksSchemaAndPrecompilesMatcher(t *testing.T) {
	entry := testEntry("pkg/rule", "prompt.submitted", "event", "inject", "body", 1)
	index := testIndex(map[string][]compile.Entry{"prompt.submitted": {entry}})
	data, err := compile.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t)
	engine, err := Load(bytes.NewReader(data), store)
	if err != nil {
		t.Fatalf("Load(valid) = %v", err)
	}
	output, err := engine.Process(testInput("prompt.submitted"), tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "body" {
		t.Fatalf("loaded engine output = %#v, %v", output, err)
	}
	for _, invalid := range [][]byte{
		[]byte("{"),
		[]byte("{\"format\":2}"),
		append(append([]byte(nil), data...), []byte("{}")...),
	} {
		if _, err := Load(bytes.NewReader(invalid), newTestStore(t)); err == nil {
			t.Errorf("Load accepted invalid index: %s", invalid)
		}
	}
	if _, err := Load(bytes.NewReader(bytes.Repeat([]byte("x"), maxIndexBytes+1)), newTestStore(t)); err == nil {
		t.Fatal("Load accepted oversized index")
	}
}

func TestTargetConditionsAndSessionSourceAreApplied(t *testing.T) {
	tool := testEntry("pkg/tool", "tool.pre", "tool", "inject", "matched", 1)
	tool.Match = &compile.Match{Canonical: []string{"shell"}, Names: []string{"Bash"}}
	tool.When.Conditions = []when.Condition{{Field: "tool.input.command", Op: "contains", Pattern: "git push"}}
	session := testEntry("pkg/session", "session.start", "event", "inject", "startup only", 2)
	sources := []string{"startup"}
	session.Sources = &sources
	engine := newTestEngine(t, map[string][]compile.Entry{"tool.pre": {tool}, "session.start": {session}})
	input := testInput("tool.pre")
	input.Points[0].CanonicalTool, input.Points[0].ToolName = "shell", "Bash"
	input.Points[0].When.Fields = when.FieldMap{"tool.input.command": "git push origin main"}
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "matched" {
		t.Fatalf("matching tool output = %#v, %v", output, err)
	}
	input.Points[0].When.Fields = when.FieldMap{}
	if output, err = engine.Process(input, tokens.InputLimit{}); err != nil || output.AdditionalContext != "" {
		t.Fatalf("missing condition field matched: %#v, %v", output, err)
	}
	input = testInput("session.start")
	input.Source = "resume"
	if output, err = engine.Process(input, tokens.InputLimit{}); err != nil || output.AdditionalContext != "" {
		t.Fatalf("resume matched startup-only entry: %#v, %v", output, err)
	}
	input.Source = "startup"
	if output, err = engine.Process(input, tokens.InputLimit{}); err != nil || output.AdditionalContext != "startup only" {
		t.Fatalf("startup output = %#v, %v", output, err)
	}
}

func TestBlockTakesPrecedenceAndDoesNotClaimOtherEntries(t *testing.T) {
	block := testEntry("pkg/block", "prompt.submitted", "event", "block", "blocked by policy", 1)
	inject := testEntry("pkg/inject", "prompt.submitted", "event", "inject", "context", 2)
	warn := testEntry("pkg/warn", "prompt.submitted", "event", "warn", "full warning", 3)
	modelBody := "model warning"
	warn.ModelBody = &compile.Body{Static: &modelBody}
	engine := newTestEngine(t, map[string][]compile.Entry{"prompt.submitted": {inject, warn, block}})
	input := testInput("prompt.submitted")
	input.Points[0].Capabilities = Capabilities{AddContext: true, SystemMessage: true, Deny: true}
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if !output.Block || output.Reason != "blocked by policy" || output.AdditionalContext != "" || output.SystemMessage != "" || output.SuppressOriginalPrompt {
		t.Fatalf("block output included non-block channels: %#v", output)
	}
	reservation, allowed, err := engine.store.Claim(state.ClaimInput{
		Scope: input.Scope, Repeat: "session", Action: "inject", Event: "prompt.submitted", Rule: inject.Rule, Body: "context",
	})
	if err != nil || !allowed || reservation == nil {
		t.Fatalf("block path reserved inject: %#v, %v, %v", reservation, allowed, err)
	}
	_ = reservation.Release()
}

func TestWarnMergeUsesCapabilitiesAndFirstSentence(t *testing.T) {
	warn := testEntry("pkg/warn", "tool.post", "event", "warn", "First sentence. Second sentence.", 1)
	warn.ModelBodyFirstSentence = true
	engine := newTestEngine(t, map[string][]compile.Entry{"tool.post": {warn}})
	input := testInput("tool.post")
	input.Points[0].Capabilities = Capabilities{AddContext: true, SystemMessage: true}
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if output.AdditionalContext != "First sentence." || output.SystemMessage != "First sentence. Second sentence." {
		t.Fatalf("warn output = %#v", output)
	}
	_ = output.Reservations[0].Release()

	limited := testInput("tool.post")
	limited.Points[0].Capabilities = Capabilities{SystemMessage: true}
	output, err = engine.Process(limited, tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if output.AdditionalContext != "" || output.SystemMessage != "First sentence. Second sentence." {
		t.Fatalf("system-message-only warn output = %#v", output)
	}
}

func TestWarnRepeatEverySendsEveryMatchingInvocation(t *testing.T) {
	warn := testEntry("pkg/warn", "tool.post", "event", "warn", "Full warning.", 1)
	warn.Repeat = "every"
	warn.ModelBodyFirstSentence = true
	engine := newTestEngine(t, map[string][]compile.Entry{"tool.post": {warn}})
	input := testInput("tool.post")
	input.Points[0].Capabilities.SystemMessage = true
	for i := 0; i < 2; i++ {
		output, err := engine.Process(input, tokens.InputLimit{})
		if err != nil || output.SystemMessage != "Full warning." || output.AdditionalContext != "Full warning." {
			t.Fatalf("warn invocation %d = %#v, %v", i, output, err)
		}
		if len(output.Reservations) != 0 {
			t.Fatalf("repeat: every created reservations: %#v", output.Reservations)
		}
	}
}

func TestInjectAndWarnEntriesMergeInOrder(t *testing.T) {
	first := testEntry("pkg/first", "prompt.submitted", "event", "inject", "first inject", 1)
	second := testEntry("pkg/second", "prompt.submitted", "event", "inject", "second inject", 2)
	warning := testEntry("pkg/warn", "prompt.submitted", "event", "warn", "Full warning. More details.", 3)
	warning.ModelBodyFirstSentence = true
	core := newTestEngine(t, map[string][]compile.Entry{"prompt.submitted": {warning, second, first}})
	input := testInput("prompt.submitted")
	input.Points[0].Capabilities.SystemMessage = true
	output, err := core.Process(input, tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if output.AdditionalContext != "first inject\n\nsecond inject\n\nFull warning." {
		t.Fatalf("model context order = %q", output.AdditionalContext)
	}
	if output.SystemMessage != "Full warning. More details." {
		t.Fatalf("user warning = %q", output.SystemMessage)
	}
	for _, reservation := range output.Reservations {
		_ = reservation.Release()
	}
}

func TestCapDropsHighestOrderAndReleasesItsReservation(t *testing.T) {
	entries := []compile.Entry{
		testEntry("pkg/low", "prompt.submitted", "event", "inject", "a", 1),
		testEntry("pkg/mid", "prompt.submitted", "event", "inject", "bb", 2),
		testEntry("pkg/high", "prompt.submitted", "event", "inject", "ccc", 3),
	}
	engine := newTestEngine(t, map[string][]compile.Entry{"prompt.submitted": entries})
	input := testInput("prompt.submitted")
	output, err := engine.Process(input, tokens.InputLimit{Value: 6, Unit: tokens.CharsUnit})
	if err != nil {
		t.Fatal(err)
	}
	capDropFound := false
	for _, record := range output.Records {
		if record.Result == "cap-drop" && record.Entry == "pkg/high@prompt.submitted" {
			capDropFound = true
		}
	}
	if output.AdditionalContext != "a\n\nbb" || len(output.Records) != 3 || !capDropFound {
		t.Fatalf("capped output = %#v", output)
	}
	output, err = engine.Process(input, tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if output.AdditionalContext != "ccc" {
		t.Fatalf("dropped entry reservation was not released: %#v", output)
	}
	for _, reservation := range output.Reservations {
		_ = reservation.Release()
	}
}

func TestDynamicSystemReevaluatesAndDeduplicatesByRenderedBody(t *testing.T) {
	start := testEntry("pkg/system", "session.start", "system", "inject", "", 1)
	start.Sources = stringSlice("startup", "compact")
	start.ReinjectOnCompact = true
	start.Body = dynamicModelBody("model=")
	prompt := testEntry("pkg/system", "prompt.submitted", "system", "inject", "", 1)
	prompt.Reevaluate = true
	prompt.ReinjectOnCompact = true
	prompt.Body = dynamicModelBody("model=")
	engine := newTestEngine(t, map[string][]compile.Entry{"session.start": {start}, "prompt.submitted": {prompt}})
	store := engine.store
	_, scope, err := store.Update("session", "main", state.EventUpdate{Source: "startup", Model: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	input := testInput("session.start")
	input.Scope, input.Source = scope, "startup"
	input.Variables["model"] = "alpha"
	input.Points[0].When.Model = "alpha"
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "model=alpha" {
		t.Fatalf("session start dynamic output = %#v, %v", output, err)
	}
	_, scope, err = store.Update("session", "main", state.EventUpdate{PromptSubmitted: true, Model: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	input = testInput("prompt.submitted")
	input.Scope, input.TurnID = scope, "turn-1"
	input.Variables["model"] = "alpha"
	input.Points[0].When.Model = "alpha"
	output, err = engine.Process(input, tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "" {
		t.Fatalf("unchanged system body repeated: %#v, %v", output, err)
	}
	_, scope, err = store.Update("session", "main", state.EventUpdate{PromptSubmitted: true, Model: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	input.Scope = scope
	input.Variables["model"] = "beta"
	input.Points[0].When.Model = "beta"
	output, err = engine.Process(input, tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "model=beta" {
		t.Fatalf("changed system body was not reinjected: %#v, %v", output, err)
	}
	for _, reservation := range output.Reservations {
		_ = reservation.Release()
	}
}

func TestClaudePromptBlockSetsOnlyAllowedSuppressField(t *testing.T) {
	entry := testEntry("pkg/block", "prompt.submitted", "event", "block", "no", 1)
	engine := newTestEngine(t, map[string][]compile.Entry{"prompt.submitted": {entry}})
	input := testInput("prompt.submitted")
	input.Harness = "claude-code"
	input.Points[0].Capabilities.Deny = true
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if !output.Block || !output.SuppressOriginalPrompt || output.Validate() != nil {
		t.Fatalf("Claude prompt block = %#v", output)
	}
	if err := (Output{Reason: "not blocking"}).Validate(); err == nil {
		t.Fatal("output whitelist accepted a block-only field without block")
	}
	if err := (Output{Block: true, Reason: "deny", AdditionalContext: "leak"}).Validate(); err == nil {
		t.Fatal("output whitelist accepted context alongside block")
	}
	if err := (Output{Block: true, Reason: "deny", SuppressOriginalPrompt: true}).ValidateFor("codex"); err == nil {
		t.Fatal("output whitelist accepted Claude-only suppression on Codex")
	}
}

func TestPromptPointForMainAndSubagent(t *testing.T) {
	entry := testEntry("pkg/main", "prompt.submitted", "event", "inject", "main", 1)
	engine := newTestEngine(t, map[string][]compile.Entry{"prompt.submitted": {entry}})
	input := testInput("prompt.submitted")
	input.Points[0].When.AgentID = ""
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "main" {
		t.Fatalf("main agent did not match default: %#v, %v", output, err)
	}
	for _, reservation := range output.Reservations {
		_ = reservation.Release()
	}
	input.Scope.AgentID = "child-1"
	input.Points[0].When.AgentID, input.Points[0].When.AgentType = "child-1", "reviewer"
	if output, err := engine.Process(input, tokens.InputLimit{}); err != nil || output.AdditionalContext != "" {
		t.Fatalf("main-only rule matched subagent: %#v, %v", output, err)
	}
}

func newTestEngine(t *testing.T, events map[string][]compile.Entry) *Engine {
	t.Helper()
	engine, err := New(testIndex(events), newTestStore(t))
	if err != nil {
		t.Fatalf("New(test index): %v", err)
	}
	return engine
}

func newTestStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testIndex(events map[string][]compile.Entry) compile.Index {
	return compile.Index{
		Format: 1, StyleVersion: "0.3.0", Packages: []compile.PackageInfo{},
		Static:    map[string][]compile.StaticEntry{"codex": {}, "claude-code": {}},
		Harnesses: map[string]map[string][]compile.Entry{"codex": events, "claude-code": events},
	}
}

func testEntry(rule, point, target, action, body string, priority float64) compile.Entry {
	entry := compile.Entry{
		Rule: rule, Entry: rule + "@" + point, Source: "rules/test.md", SourceSHA256: testSHA, BodySHA256: testSHA,
		Order: compile.Order{Priority: priority, PackageSeq: 1}, Target: target, Action: action,
		When:   when.Rule{ThinkingEffortApprox: boolPtr(false), Agent: []string{"main"}, Event: point},
		Repeat: "session", Needs: []string{}, Body: compile.Body{Static: &body}, Estimate: tokens.Estimate{},
	}
	if point == "session.start" {
		entry.Sources = stringSlice("startup", "clear", "fork", "compact")
	}
	if action == "warn" {
		entry.ModelBody = &compile.Body{Static: &body}
	}
	return entry
}

func testInput(point string) Input {
	return Input{Harness: "codex", Source: "startup", Scope: state.Scope{SessionID: "session", AgentID: "main"},
		Variables: map[string]string{}, Points: []Point{{Name: point, When: when.Input{ThinkingEffortApprox: boolPtr(false)},
			Capabilities: Capabilities{AddContext: true, Deny: true}}}}
}

func boolPtr(value bool) *bool { return &value }

func stringSlice(values ...string) *[]string { return &values }

func dynamicModelBody(prefix string) compile.Body {
	return compile.Body{Parts: []compile.Fragment{{Text: prefix}, {Variable: &compile.Variable{Name: "model"}}}}
}

func TestRenderValuesFromFieldsAreAvailableToTemplates(t *testing.T) {
	entry := testEntry("pkg/input", "tool.pre", "event", "inject", "{{tool.input.command}}/{{harness.cwd}}", 1)
	entry.Body = compile.Body{Parts: []compile.Fragment{
		{Variable: &compile.Variable{Name: "tool.input.command"}},
		{Text: "/"},
		{Variable: &compile.Variable{Name: "harness.cwd"}},
	}}
	engine := newTestEngine(t, map[string][]compile.Entry{"tool.pre": {entry}})
	input := testInput("tool.pre")
	input.Points[0].Values = map[string]string{"tool.input.command": "git status"}
	input.Points[0].When.Harness = map[string]string{"cwd": "/repo"}
	output, err := engine.Process(input, tokens.InputLimit{})
	if err != nil || !strings.Contains(output.AdditionalContext, "git status//repo") {
		t.Fatalf("dynamic input output = %#v, %v", output, err)
	}
}

const engineProcessRootEnv = "STYLE_ENGINE_PROCESS_ROOT"

func TestEngineProcessHelper(t *testing.T) {
	root := os.Getenv(engineProcessRootEnv)
	if root == "" {
		return
	}
	store, err := state.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := testEntry("pkg/shared", "prompt.submitted", "event", "inject", "shared context", 1)
	core, err := New(testIndex(map[string][]compile.Entry{"prompt.submitted": {entry}}), store)
	if err != nil {
		t.Fatal(err)
	}
	output, err := core.Process(testInput("prompt.submitted"), tokens.InputLimit{})
	if err != nil {
		t.Fatal(err)
	}
	if output.AdditionalContext != "" {
		t.Log("STYLE_ENGINE_OUTPUT")
	}
}

func TestTwentyProcessesEmitOneRepeatSessionEntry(t *testing.T) {
	root := t.TempDir()
	const workers = 20
	start := make(chan struct{})
	ready := sync.WaitGroup{}
	results := make(chan struct {
		output []byte
		err    error
	}, workers)
	for i := 0; i < workers; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestEngineProcessHelper$", "-test.v")
		cmd.Env = engineTestEnvWith(os.Environ(), engineProcessRootEnv, root)
		ready.Add(1)
		go func(cmd *exec.Cmd) {
			ready.Done()
			<-start
			output, err := cmd.CombinedOutput()
			results <- struct {
				output []byte
				err    error
			}{output: output, err: err}
		}(cmd)
	}
	ready.Wait()
	close(start)
	wins := 0
	for i := 0; i < workers; i++ {
		result := <-results
		if result.err != nil {
			t.Errorf("engine process failed: %v\n%s", result.err, result.output)
		}
		if strings.Contains(string(result.output), "STYLE_ENGINE_OUTPUT") {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("process output winners = %d, want exactly one", wins)
	}
}

func engineTestEnvWith(environment []string, name, value string) []string {
	key := name + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, key) {
			filtered = append(filtered, item)
		}
	}
	return append(filtered, key+value)
}

func TestEngineProcessLatencyP95Below50ms(t *testing.T) {
	core, input := indexOnlyEngine(t)
	const samples = 200
	durations := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		started := time.Now()
		if _, err := core.Process(input, tokens.InputLimit{}); err != nil {
			t.Fatal(err)
		}
		durations = append(durations, time.Since(started))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(samples*95+99)/100-1]
	t.Logf("index-only engine Process p95 = %s (samples=%d)", p95, samples)
	if p95 >= 50*time.Millisecond {
		t.Fatalf("index-only engine Process p95 = %s, want <50ms", p95)
	}
}

func BenchmarkEngineProcessIndexOnly(b *testing.B) {
	core, input := indexOnlyEngine(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := core.Process(input, tokens.InputLimit{}); err != nil {
			b.Fatal(err)
		}
	}
}

func indexOnlyEngine(tb testing.TB) (*Engine, Input) {
	tb.Helper()
	store, err := state.New(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	entries := make([]compile.Entry, 24)
	for i := range entries {
		body := compile.Body{Parts: []compile.Fragment{
			{Text: "Review "}, {Variable: &compile.Variable{Name: "model"}}, {Text: " for safe edits."},
		}}
		entries[i] = compile.Entry{
			Rule: "pkg/rule-" + strconv.Itoa(i), Entry: "pkg/rule-" + strconv.Itoa(i) + "@prompt.submitted",
			Source: "rules/test.md", SourceSHA256: testSHA, BodySHA256: testSHA,
			Order: compile.Order{Priority: float64(i), PackageSeq: 1, RuleIndex: i}, Target: "event", Action: "inject",
			When:   when.Rule{ThinkingEffortApprox: boolPtr(false), Agent: []string{"main"}},
			Repeat: "every", Needs: []string{}, Body: body, Estimate: tokens.Estimate{}, DegradedFrom: nil,
		}
	}
	core, err := New(testIndex(map[string][]compile.Entry{"prompt.submitted": entries}), store)
	if err != nil {
		tb.Fatal(err)
	}
	input := testInput("prompt.submitted")
	input.Variables["model"] = "gpt-test"
	return core, input
}
