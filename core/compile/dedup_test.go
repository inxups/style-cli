package compile

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
)

const dedupRules = `  - id: static
    target: system
    text: 'Use {{settings.voice}}.'
  - id: dynamic
    target: system
    text: 'Model {{model}}.'
  - id: skill
    target: skill
    skill: Review
    text: Review carefully.
  - id: prompt
    target: event
    event: prompt.submitted
    text: Prompt guidance.
  - id: warning
    target: event
    event: tool.post
    action: warn
    text: 'Warning {{model}}. More detail.'
  - id: block
    target: tool
    tool: shell
    at: before
    action: block
    text: Denied.
  - id: child
    target: event
    event: subagent.start
    text: Child guidance.
`

func TestCrossPackageDedupKeepsWholeRulesAndStableCanonical(t *testing.T) {
	var inputs []PackageInput
	for i, name := range []string{"first", "second", "third"} {
		pkg := loadResolved(t, name, dedupRules)
		for j := range pkg.Rules {
			pkg.Rules[j].Effective["priority"] = []float64{25.5, 25.25, 25.25}[i]
		}
		input := inputFor(t, pkg, i+1)
		input.Plans = append(input.Plans, makePlan(t, pkg, "other-harness"))
		inputs = append(inputs, input)
	}
	before, _ := json.Marshal(inputs)
	build := func(inputs []PackageInput) Result {
		t.Helper()
		result, err := Build(Request{StyleVersion: "test", Packages: inputs})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := build(inputs)
	if len(result.Index.Packages) != 3 || len(result.StaticBodies) != 2 || len(result.Deduplicated) != 28 || len(result.DeduplicationWarnings) != 0 {
		t.Fatalf("wrong compilation sizes: packages=%d static=%d aliases=%d warnings=%v", len(result.Index.Packages), len(result.StaticBodies), len(result.Deduplicated), result.DeduplicationWarnings)
	}
	for harness, events := range result.Index.Harnesses {
		count := 0
		for _, entries := range events {
			for _, entry := range entries {
				count++
				if !strings.HasPrefix(entry.Rule, "second/") || entry.Order.Priority != 25.25 || entry.Order.PackageSeq != 2 {
					t.Fatalf("wrong canonical: %+v", entry)
				}
			}
		}
		if count != 8 || len(result.Index.Static[harness]) != 1 || result.Index.Static[harness][0].Rule != "second/static" {
			t.Fatalf("missing whole-rule placement: %s %v", harness, events)
		}
	}
	for _, relation := range result.Deduplicated {
		if !strings.HasPrefix(relation.Into, "second/") || len(relation.Placements) == 0 {
			t.Fatalf("bad relation: %+v", relation)
		}
		if strings.HasSuffix(relation.Rule, "/dynamic") || strings.HasSuffix(relation.Rule, "/skill") {
			if len(relation.Placements) != 2 {
				t.Fatalf("entry points were split: %+v", relation)
			}
		}
	}
	slices.Reverse(inputs)
	reversed := build(inputs)
	if !reflect.DeepEqual(result, reversed) {
		t.Fatal("input traversal changed canonical selection or report ordering")
	}
	slices.Reverse(inputs)
	after, _ := json.Marshal(inputs)
	if string(before) != string(after) {
		t.Fatal("compile mutated captured package/plan inputs")
	}
	encoded, err := Encode(result.Index)
	if err != nil || strings.Contains(string(encoded), "deduplicated") {
		t.Fatalf("cold-path alias leaked into index: %v", err)
	}
	var roundTrip Index
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("format 1 round trip: %v", err)
	}
}

func TestCrossPackageDedupAmbiguousSamePackagePreservesEntireClass(t *testing.T) {
	first := inputFor(t, loadResolved(t, "first", `  - {id: a, target: event, event: prompt.submitted, text: Same.}
  - {id: b, target: event, event: prompt.submitted, text: Same.}
`), 1)
	second := inputFor(t, loadResolved(t, "second", `  - {id: c, target: event, event: prompt.submitted, text: Same.}
`), 2)
	result, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{second, first}})
	if err != nil {
		t.Fatal(err)
	}
	entries := result.Index.Harnesses["codex"][plan.PromptSubmitted]
	if len(entries) != 3 || len(result.Deduplicated) != 0 || len(result.DeduplicationWarnings) != 3 {
		t.Fatalf("ambiguous class was collapsed: %+v", result)
	}
	if entries[0].Rule != "first/a" || entries[1].Rule != "first/b" || entries[2].Rule != "second/c" {
		t.Fatalf("rule index ordering lost: %+v", entries)
	}
	for _, warning := range result.DeduplicationWarnings {
		if warning.Reason != "multiple equivalent rules in one package" {
			t.Fatalf("unexplained preservation: %+v", warning)
		}
	}
}

func TestCrossPackageDedupUsesRenderedStaticBytes(t *testing.T) {
	cases := []struct {
		name, left, right, voice string
		merge                    bool
	}{
		{"settings and literal", "Use {{settings.voice}}.", "Use precise.", "precise", true},
		{"different settings", "Use {{settings.voice}}.", "Use precise.", "other", false},
		{"package variable", "Use {{package.name}}.", "Use {{package.name}}.", "precise", false},
		{"rule variable", "Use {{rule.id}}.", "Use r.", "precise", true},
		{"different whitespace", "Use precise.", "Use  precise.", "precise", false},
		{"different Unicode", "é", "é", "precise", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mk := func(name, body string, seq int) PackageInput {
				return inputFor(t, loadResolved(t, name, "  - {id: r, target: system, text: '"+body+"'}\n"), seq)
			}
			left, right := mk("left", test.left, 1), mk("right", test.right, 2)
			left.Settings["voice"] = test.voice
			result, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{left, right}})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(result.Deduplicated) == 1; got != test.merge || len(result.StaticBodies) != 2-len(result.Deduplicated) {
				t.Fatalf("merge=%v want=%v result=%+v", got, test.merge, result)
			}
		})
	}
}

// Compare real compiled entries with one behavior changed at a time. These
// cases distinguish equality of emitted text from equality of future behavior.
func TestDedupPreservesDifferentCompiledSemantics(t *testing.T) {
	base := inputFor(t, loadResolved(t, "first", `  - id: r
    target: tool
    tool: shell
    at: before
    text: 'Model {{model | default: "X"}}.'
`), 1)
	single, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{base}})
	if err != nil {
		t.Fatal(err)
	}
	original := single.Index.Harnesses["codex"][plan.ToolPre][0]
	cases := []struct {
		name   string
		change func(*Entry)
	}{
		{"action", func(e *Entry) { e.Action = "warn" }},
		{"target", func(e *Entry) { e.Target = "mcp" }},
		{"tool", func(e *Entry) { e.Match = &Match{Canonical: []string{"file.edit"}} }},
		{"native tool name", func(e *Entry) { e.Match = &Match{Names: []string{"Bash"}} }},
		{"MCP", func(e *Entry) { e.Match = &Match{MCP: []MCPSelector{{Server: "s", Tool: "t"}}} }},
		{"skill", func(e *Entry) { e.Match = &Match{Skill: []string{"review"}} }},
		{"condition", func(e *Entry) { e.When.Conditions = []when.Condition{{Field: "cwd", Op: "contains", Pattern: "/repo"}} }},
		{"model", func(e *Entry) { e.When.Model = []string{"gpt-*"} }},
		{"empty model alternatives", func(e *Entry) { e.When.Model = []string{} }},
		{"agent", func(e *Entry) { e.When.Agent = []string{"*"} }},
		{"repeat", func(e *Entry) { e.Repeat = "every" }},
		{"sources", func(e *Entry) { v := []string{"startup"}; e.Sources = &v }},
		{"reinject", func(e *Entry) { e.ReinjectOnCompact = !e.ReinjectOnCompact }},
		{"reevaluate", func(e *Entry) { e.Reevaluate = !e.Reevaluate }},
		{"needs", func(e *Entry) { e.Needs = []string{"thinking_effort"} }},
		{"variable", func(e *Entry) {
			v := *e.Body.Parts[1].Variable
			v.Name = "thinking_effort"
			e.Body.Parts[1].Variable = &v
		}},
		{"default", func(e *Entry) {
			v := *e.Body.Parts[1].Variable
			s := "Y"
			v.Default = &s
			e.Body.Parts[1].Variable = &v
		}},
		{"no default", func(e *Entry) { v := *e.Body.Parts[1].Variable; v.Default = nil; e.Body.Parts[1].Variable = &v }},
		{"runtime coincidence", func(e *Entry) { s := "Model X."; e.Body = Body{Static: &s} }},
		{"trailing newline", func(e *Entry) { e.Body.Parts = append(e.Body.Parts, Fragment{Text: "\n"}) }},
		{"warn model channel", func(e *Entry) { s := "Model only."; e.ModelBody = &Body{Static: &s} }},
		{"warn first sentence", func(e *Entry) { e.ModelBodyFirstSentence = true }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			left, right := original, original
			right.Rule, right.Entry, right.Order.PackageSeq = "second/r", "second/r@tool.pre", 2
			right.Body.Parts = append([]Fragment(nil), original.Body.Parts...)
			test.change(&right)
			result := Result{Index: Index{Harnesses: map[string]map[string][]Entry{"codex": {plan.ToolPre: {left, right}}}}}
			if err := deduplicate(&result, nil); err != nil {
				t.Fatal(err)
			}
			if len(result.Index.Harnesses["codex"][plan.ToolPre]) != 2 || len(result.Deduplicated) != 0 {
				t.Fatalf("different semantics were folded: %+v", result)
			}
		})
	}
}

func TestCrossPackageDedupRequiresSameCompletePlacementSetAndCapabilities(t *testing.T) {
	for _, test := range []string{"entry point", "capability", "skipped", "never", "different harness", "static mode", "static slot", "static fallback prefix"} {
		t.Run(test, func(t *testing.T) {
			rules := "  - {id: r, target: system, text: 'Model {{model}}.'}\n"
			if strings.HasPrefix(test, "static") {
				rules = "  - {id: r, target: system, text: Same.}\n"
			}
			left := inputFor(t, loadResolved(t, "left", rules), 1)
			right := inputFor(t, loadResolved(t, "right", rules), 2)
			p := &left.Plans[0].Rules[0]
			switch test {
			case "entry point":
				p.Placements = p.Placements[:1]
			case "capability":
				p.Placements[0].Capabilities = []plan.Capability{plan.SystemMessage}
			case "skipped":
				for i := range p.Placements {
					p.Placements[i].Status = plan.Skipped
				}
			case "never":
				for i := range p.Placements {
					p.Placements[i].Status = plan.Never
				}
			case "different harness":
				left.Plans[0].Adapter = "other"
			case "static mode":
				left.Package.Rules[0].Effective["mode"] = "replace"
			case "static slot":
				p.Placements[0].StaticSlot = "claude_md"
			case "static fallback prefix":
				p.Placements[0].StaticFallback = true
				p.Placements[0].StaticPrefix = "Fallback: "
			}
			result, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{left, right}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Deduplicated) != 0 {
				t.Fatalf("ineligible rules were folded: %+v", result.Deduplicated)
			}
		})
	}
}

func TestDedupIgnoresProvenanceAndEstimates(t *testing.T) {
	first := inputFor(t, loadResolved(t, "first", "  - {id: r, target: event, event: prompt.submitted, text: Same.}\n"), 1)
	result, err := Build(Request{StyleVersion: "test", Packages: []PackageInput{first}})
	if err != nil {
		t.Fatal(err)
	}
	entries := result.Index.Harnesses["codex"][plan.PromptSubmitted]
	other := entries[0]
	other.Rule, other.Entry, other.Source, other.SourceSHA256, other.BodySHA256 = "second/alias", "second/alias@prompt.submitted", "different.md", "different", "different"
	other.Order.PackageSeq, other.Estimate = 2, tokens.Estimate{Tokens: 999, Chars: 999}
	s := "block"
	other.DegradedFrom = &s
	result.Index.Harnesses["codex"][plan.PromptSubmitted] = append(entries, other)
	if err := deduplicate(&result, nil); err != nil {
		t.Fatal(err)
	}
	if len(result.Deduplicated) != 1 || result.Deduplicated[0].Into != "first/r" || !reflect.DeepEqual(result.Index.Harnesses["codex"][plan.PromptSubmitted][0], entries[0]) {
		t.Fatal("provenance blocked folding or canonical entry was rewritten")
	}
}
