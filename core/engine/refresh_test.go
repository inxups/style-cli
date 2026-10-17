package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
)

func refreshEngine(t *testing.T, events map[string][]compile.Entry) *Engine {
	t.Helper()
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	index := testIndex(events)
	index.Format = 2
	core, err := New(index, newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func TestRefreshRejectsMismatchedFrame(t *testing.T) {
	core := refreshEngine(t, map[string][]compile.Entry{})
	g, err := core.store.BeginRefresh(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	_, scope, err := core.store.UpdateLocked("session", "main", state.EventUpdate{}, g)
	if err != nil {
		t.Fatal(err)
	}
	f, err := g.Advance(scope, "claudecode", state.EventUpdate{}, "")
	if err != nil {
		t.Fatal(err)
	}
	in := testInput("tool.pre")
	in.Scope, in.Refresh = scope, f
	if _, err := core.Process(in, tokens.InputLimit{}); err == nil {
		t.Fatal("accepted another harness's clock")
	}
}
func refreshProcess(t *testing.T, core *Engine, update state.EventUpdate, id string, points []string, limit tokens.InputLimit, edit func(*Input)) (Output, Trace) {
	t.Helper()
	g, err := core.store.BeginRefresh(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	_, scope, err := core.store.UpdateLocked("session", "main", update, g)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := g.Advance(scope, "codex", update, id)
	if err != nil {
		t.Fatal(err)
	}
	input := testInput(points[0])
	input.Scope = scope
	input.Refresh = frame
	input.TurnID = id
	input.TurnSource = "native"
	input.Source = update.Source
	input.Points = nil
	for _, name := range points {
		point := testInput(name).Points[0]
		input.Points = append(input.Points, point)
	}
	if edit != nil {
		edit(&input)
	}
	output, trace, err := core.ProcessWithTrace(input, limit)
	if err != nil {
		t.Fatal(err)
	}
	return output, trace
}

func TestRefreshCapRollbackAndBlockPrecedence(t *testing.T) {
	entry := testEntry("pkg/r", "tool.pre", "event", "inject", "periodic guidance", 1)
	entry.Refresh = 3
	core := refreshEngine(t, map[string][]compile.Entry{"tool.pre": {entry}})
	drop, tr := refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.pre"}, tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}, nil)
	if drop.AdditionalContext != "" || tr.Entries[0].Refresh.Reason != "cap-release" || !tr.Entries[0].ReservationReleased {
		t.Fatal(drop, tr.Entries[0])
	}
	out, _ := refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.pre"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext != "periodic guidance" {
		t.Fatal(out)
	}
	for i := 1; i <= 3; i++ {
		refreshProcess(t, core, state.EventUpdate{PromptSubmitted: true}, fmt.Sprint(i), []string{"prompt.submitted"}, tokens.InputLimit{}, nil)
	}
	_, tr = refreshProcess(t, core, state.EventUpdate{}, "3", []string{"tool.pre"}, tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}, nil)
	if tr.Entries[0].Refresh.Last == nil || *tr.Entries[0].Refresh.Last != 0 {
		t.Fatal(tr.Entries[0])
	}
	out, tr = refreshProcess(t, core, state.EventUpdate{}, "3", []string{"tool.pre"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext == "" || tr.Entries[0].Refresh.Reason != "due" || *tr.Entries[0].Refresh.Last != 0 {
		t.Fatal(out, tr.Entries[0])
	}
	block := testEntry("pkg/block", "tool.pre", "event", "block", "stop", 0)
	core = refreshEngine(t, map[string][]compile.Entry{"tool.pre": {entry, block}})
	out, _ = refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.pre"}, tokens.InputLimit{}, func(in *Input) { in.Points[0].Capabilities.Deny = true })
	if !out.Block {
		t.Fatal(out)
	}
	delete(core.entries["codex"], "tool.pre")
	core.entries["codex"]["tool.pre"] = []compiledEntry{{entry: entry, matcher: mustRefreshMatcher(t, entry.When)}}
	out, _ = refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.pre"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext == "" {
		t.Fatal("block consumed periodic chance")
	}
}
func mustRefreshMatcher(t *testing.T, r when.Rule) *when.Matcher {
	t.Helper()
	m, err := when.Compile(r)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRefreshRenderedBodiesWarnAndMultiplePoints(t *testing.T) {
	entry := testEntry("pkg/r", "tool.pre", "event", "inject", "", 1)
	entry.Refresh = 3
	entry.Body = compile.Body{Parts: []compile.Fragment{{Variable: &compile.Variable{Name: "model"}}}}
	second := entry
	second.Entry = "pkg/r@tool.post"
	core := refreshEngine(t, map[string][]compile.Entry{"tool.pre": {entry}, "tool.post": {second}})
	for _, tc := range []struct {
		body string
		want bool
	}{{"", false}, {"A", true}, {"B", true}, {"A", false}} {
		out, _ := refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.pre", "tool.post"}, tokens.InputLimit{}, func(in *Input) { in.Variables = map[string]string{"model": tc.body} })
		if (out.AdditionalContext != "") != tc.want || strings.Count(out.AdditionalContext, tc.body) > 1 && tc.body != "" {
			t.Fatal(tc, out)
		}
	}
	for i := 1; i <= 3; i++ {
		refreshProcess(t, core, state.EventUpdate{PromptSubmitted: true}, fmt.Sprint(i), []string{"prompt.submitted"}, tokens.InputLimit{}, nil)
	}
	out, tr := refreshProcess(t, core, state.EventUpdate{}, "3", []string{"tool.pre", "tool.post"}, tokens.InputLimit{}, func(in *Input) { in.Variables = map[string]string{"model": "A"} })
	if out.AdditionalContext != "A" || len(out.Records) != 2 || out.Records[0].Result != "dedup" {
		t.Fatal(out, tr)
	}
	warn := testEntry("pkg/w", "tool.post", "event", "warn", "First warning. Detail.", 1)
	warn.Refresh = 1
	warn.ModelBodyFirstSentence = true
	core = refreshEngine(t, map[string][]compile.Entry{"tool.post": {warn}})
	out, _ = refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.post"}, tokens.InputLimit{}, func(in *Input) { in.Points[0].Capabilities = Capabilities{SystemMessage: true} })
	if out.SystemMessage != "First warning. Detail." || out.AdditionalContext != "" {
		t.Fatal(out)
	}
	out, _ = refreshProcess(t, core, state.EventUpdate{}, "", []string{"tool.post"}, tokens.InputLimit{}, func(in *Input) { in.Points[0].Capabilities = Capabilities{SystemMessage: true} })
	if out.SystemMessage != "" {
		t.Fatal(out)
	}
	out, _ = refreshProcess(t, core, state.EventUpdate{PromptSubmitted: true}, "1", []string{"tool.post"}, tokens.InputLimit{}, func(in *Input) { in.Points[0].Capabilities = Capabilities{SystemMessage: true} })
	if out.SystemMessage == "" {
		t.Fatal(out)
	}
	data, _ := json.Marshal(tr)
	if strings.Contains(string(data), "periodic guidance") || strings.Contains(string(data), "/tmp/") {
		t.Fatal(string(data))
	}
}

func TestRefreshUnmatchedSourcesAndCompactFalse(t *testing.T) {
	entry := testEntry("pkg/r", "session.start", "event", "inject", "body", 1)
	entry.Refresh = 3
	entry.ReinjectOnCompact = false
	entry.Sources = stringSlice("startup", "compact")
	core := refreshEngine(t, map[string][]compile.Entry{"session.start": {entry}})
	out, _ := refreshProcess(t, core, state.EventUpdate{Source: "resume"}, "", []string{"session.start"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext != "" {
		t.Fatal(out)
	}
	out, _ = refreshProcess(t, core, state.EventUpdate{Source: "startup"}, "", []string{"session.start"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext != "body" {
		t.Fatal(out)
	}
	out, _ = refreshProcess(t, core, state.EventUpdate{Source: "compact"}, "", []string{"session.start"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext != "" {
		t.Fatal("compact false reset cursor")
	}
	for i := 1; i <= 7; i++ {
		refreshProcess(t, core, state.EventUpdate{PromptSubmitted: true}, fmt.Sprint(i), []string{"prompt.submitted"}, tokens.InputLimit{}, nil)
	}
	out, _ = refreshProcess(t, core, state.EventUpdate{Source: "resume"}, "", []string{"session.start"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext != "" {
		t.Fatal("source bypass")
	}
	out, _ = refreshProcess(t, core, state.EventUpdate{Source: "compact"}, "", []string{"session.start"}, tokens.InputLimit{}, nil)
	if out.AdditionalContext != "body" {
		t.Fatal(out)
	}
}
