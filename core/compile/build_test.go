package compile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

type compileAdapter struct{ name string }

func (adapter compileAdapter) Name() string                  { return adapter.name }
func (compileAdapter) Supports(string, plan.Capability) bool { return true }
func (compileAdapter) DataSource(string, string) bool        { return true }
func (compileAdapter) BlindSpot(spec.Rule) (string, bool)    { return "", false }
func (compileAdapter) StaticLocation(mode string) (string, bool) {
	if mode == "append" {
		return "developer_instructions", true
	}
	return "", false
}

func loadResolved(t *testing.T, name, rules string) *spec.ResolvedPackage {
	t.Helper()
	root := t.TempDir()
	manifest := "spec: style/v0.3\nname: " + name + "\nversion: 0.3.0\ndescription: compiler fixture\nreads: {model: true, thinking_effort: true, harness: [name]}\nvars: {tone: direct}\nsettings: [{name: voice, default: neutral}]\nrules:\n" + rules
	if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := spec.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := spec.Validate(loaded); hasErrors(diagnostics) {
		t.Fatalf("fixture is invalid: %#v", diagnostics)
	}
	merged, err := spec.Merge(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return merged
}

func hasErrors(diagnostics []spec.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == spec.SeverityError {
			return true
		}
	}
	return false
}

func makePlan(t *testing.T, pkg *spec.ResolvedPackage, harness string) plan.Report {
	t.Helper()
	report, err := plan.Build(pkg, compileAdapter{name: harness})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func inputFor(t *testing.T, pkg *spec.ResolvedPackage, sequence int) PackageInput {
	t.Helper()
	return PackageInput{Package: pkg, Sequence: sequence, Lock: "sha256:" + strings.Repeat("a", 64), Settings: map[string]string{"voice": "precise"}, Plans: []plan.Report{makePlan(t, pkg, "codex")}}
}

func TestBuildCompilesRuntimeAndStaticPlacements(t *testing.T) {
	pkg := loadResolved(t, "compile-test", `  - id: static
    target: system
    text: 'Keep {{vars.tone}} {{settings.voice}} {{package.name}} {{rule.id}}.'
  - id: dynamic-system
    target: system
    text: 'Model {{model | default: "unknown"}}.'
    when:
      model: [GPT-*, '**/claude-*']
      thinking_effort: '>=high'
      thinking_effort_approx: false
      harness: {name: codex}
      conditions:
        - {field: cwd, op: contains, pattern: /repo}
  - id: denied
    target: tool
    tool: shell
    at: before
    action: block
    text: 'Never force push.'
  - id: warning
    target: event
    event: tool.post
    action: warn
    text: 'Tool {{model}} failed. More detail.'
  - id: skill-rule
    target: skill
    skill: [Review, code-review]
    text: 'Review carefully.'
  - id: explicit-warning
    target: event
    event: tool.post
    action: warn
    text: 'Full details follow. A second sentence.'
    model_text: 'Use {{settings.voice}}.'
`)
	result, err := Build(Request{StyleVersion: "1.2.3", Packages: []PackageInput{inputFor(t, pkg, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Index.Format != Format || result.Index.StyleVersion != "1.2.3" || len(result.Index.Packages) != 1 {
		t.Fatalf("index header = %#v", result.Index)
	}
	if got := result.Index.Static["codex"]; len(got) != 1 || got[0].Rule != "compile-test/static" || got[0].Slot != "developer_instructions" {
		t.Fatalf("static entries = %#v", got)
	}
	if len(result.StaticBodies) != 1 || result.StaticBodies[0].Body != "Keep direct precise compile-test static." {
		t.Fatalf("rendered static body = %#v", result.StaticBodies)
	}
	entries := result.Index.Harnesses["codex"]
	if len(entries[plan.SessionStart]) != 1 || len(entries[plan.PromptSubmitted]) != 1 {
		t.Fatalf("dynamic system points = %#v", entries)
	}
	start := entries[plan.SessionStart][0]
	if start.Entry != "compile-test/dynamic-system@session.start" || start.Repeat != "session" || start.Sources == nil || !reflect.DeepEqual(*start.Sources, []string{"startup", "clear", "fork", "compact"}) {
		t.Fatalf("session entry = %#v", start)
	}
	prompt := entries[plan.PromptSubmitted][0]
	if !prompt.Reevaluate || prompt.Body.Render(nil) != "Model unknown." {
		t.Fatalf("prompt entry = %#v, rendered=%q", prompt, prompt.Body.Render(nil))
	}
	if !reflect.DeepEqual(prompt.When.Model, []string{"gpt-*", "**/claude-*"}) || !reflect.DeepEqual(prompt.When.ThinkingEffort, []string{"high", "xhigh", "max"}) || prompt.When.Harness["name"][0] != "codex" || len(prompt.When.Conditions) != 1 {
		t.Fatalf("normalized conditions = %#v", prompt.When)
	}
	if got := prompt.Body.Render(map[string]string{"model": "gpt-6"}); got != "Model gpt-6." {
		t.Fatalf("dynamic body render = %q", got)
	}
	block := entries[plan.ToolPre][0]
	if block.Action != "block" || block.Repeat != "every" || block.Match == nil || !reflect.DeepEqual(block.Match.Canonical, []string{"shell"}) {
		t.Fatalf("block entry = %#v", block)
	}
	warnEntry := entries[plan.ToolPost][0]
	if warnEntry.ModelBody == nil || !warnEntry.ModelBodyFirstSentence || warnEntry.RenderModelText(map[string]string{"model": "gpt-6"}) != "Tool gpt-6 failed." {
		t.Fatalf("dynamic warn entry = %#v; rendered=%q", warnEntry, warnEntry.RenderModelText(map[string]string{"model": "gpt-6"}))
	}
	if warnEntry.Estimate.Chars == 0 || warnEntry.Estimate.Tokens == 0 {
		t.Fatalf("warn estimate missing: %#v", warnEntry.Estimate)
	}
	var explicitWarning Entry
	for _, candidate := range entries[plan.ToolPost] {
		if candidate.Rule == "compile-test/explicit-warning" {
			explicitWarning = candidate
			break
		}
	}
	if explicitWarning.Rule == "" {
		t.Fatal("explicit warning entry is missing")
	}
	if explicitWarning.ModelBody == nil || explicitWarning.ModelBodyFirstSentence || explicitWarning.ModelBody.Render(nil) != "Use precise." {
		t.Fatalf("explicit model_text = %#v", explicitWarning)
	}
	implicit, explicit := entries[plan.ToolPost], entries[plan.SkillExplicit]
	if len(implicit) != 3 || len(explicit) != 1 || explicit[0].Rule != "compile-test/skill-rule" || !reflect.DeepEqual(explicit[0].Match.Skill, []string{"code-review", "review"}) {
		t.Fatalf("skill placements: implicit=%#v explicit=%#v", implicit, explicit)
	}

	raw, err := Encode(result.Index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) || bytes.Contains(raw, []byte("<")) {
		t.Fatalf("encoded index has wrong newline or escaped markup: %s", raw)
	}
	var decoded Index
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode emitted index: %v\n%s", err, raw)
	}
	if !reflect.DeepEqual(decoded, result.Index) {
		t.Fatalf("index round trip differs\nwant: %#v\n got: %#v", result.Index, decoded)
	}
}

func TestBuildOrderingAndEncodingAreIndependentOfInputOrder(t *testing.T) {
	first := loadResolved(t, "alpha", `  - id: later
    target: event
    event: prompt.submitted
    priority: 15
    text: Later.
  - id: first
    target: event
    event: prompt.submitted
    priority: 10.5
    text: First.
`)
	second := loadResolved(t, "beta", `  - id: other
    target: event
    event: prompt.submitted
    priority: 10
    text: Other.
`)
	one := inputFor(t, first, 2)
	two := inputFor(t, second, 1)
	left, err := Build(Request{StyleVersion: "1.0.0", Packages: []PackageInput{one, two}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := Build(Request{StyleVersion: "1.0.0", Packages: []PackageInput{two, one}})
	if err != nil {
		t.Fatal(err)
	}
	leftJSON, err := Encode(left.Index)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := Encode(right.Index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftJSON, rightJSON) {
		t.Fatalf("input order changed output\n%s\n%s", leftJSON, rightJSON)
	}
	entries := left.Index.Harnesses["codex"][plan.PromptSubmitted]
	want := []string{"beta/other", "alpha/first", "alpha/later"}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Rule)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered rules = %#v, want %#v", got, want)
	}
	if entries[1].Order.Priority != 10.5 {
		t.Fatalf("fractional priority = %v, want 10.5", entries[1].Order.Priority)
	}
	if left.Index.Packages[0].Name != "beta" || left.Index.Packages[1].Name != "alpha" {
		t.Fatalf("package ordering = %#v", left.Index.Packages)
	}
}

func TestBuildHashesLoadedSourceBytes(t *testing.T) {
	pkg := loadResolved(t, "hash-test", `  - id: source
    target: event
    event: prompt.submitted
    text: 'Hash {{vars.tone}}.'
`)
	want := sha256.Sum256(pkg.Package.Files["STYLE.yaml"])
	result, err := Build(Request{StyleVersion: "1.0.0", Packages: []PackageInput{inputFor(t, pkg, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	entry := result.Index.Harnesses["codex"][plan.PromptSubmitted][0]
	if got := entry.SourceSHA256; got != "sha256:"+hex.EncodeToString(want[:]) {
		t.Fatalf("source hash = %q", got)
	}
	if entry.BodySHA256 == entry.SourceSHA256 || !strings.HasPrefix(entry.BodySHA256, "sha256:") {
		t.Fatalf("body hash = %q, source hash = %q", entry.BodySHA256, entry.SourceSHA256)
	}
}

func TestBuildRejectsBlockedMismatchedAndIncompletePlans(t *testing.T) {
	rules := `  - id: one
    target: event
    event: prompt.submitted
    text: Body.
`
	cases := []struct {
		name string
		edit func(*PackageInput)
	}{
		{"blocked", func(in *PackageInput) { in.Plans[0].Blocked = true }},
		{"wrong package", func(in *PackageInput) { in.Plans[0].Package = "other" }},
		{"missing rule", func(in *PackageInput) { in.Plans[0].Rules = nil }},
		{"missing source bytes", func(in *PackageInput) { delete(in.Package.Package.Files, "STYLE.yaml") }},
		{"unknown evaluation point", func(in *PackageInput) { in.Plans[0].Rules[0].Placements[0].Event = "unknown.event" }},
		{"stale plan source", func(in *PackageInput) { in.Plans[0].Rules[0].Source = "rules/old.md" }},
		{"unknown action", func(in *PackageInput) { in.Plans[0].Rules[0].Placements[0].Action = "side-effect" }},
		{"unknown status", func(in *PackageInput) { in.Plans[0].Rules[0].Placements[0].Status = "future" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := inputFor(t, loadResolved(t, "invalid-plan", rules), 1)
			tc.edit(&input)
			if _, err := Build(Request{StyleVersion: "1.0.0", Packages: []PackageInput{input}}); err == nil {
				t.Fatal("Build() accepted invalid input")
			}
		})
	}
}

func TestCompileBodyRejectsUnavailableStaticValueAndKeepsEmptyDefault(t *testing.T) {
	if _, err := compileBody("{{settings.missing}}", tokenestimate.Context{}); err == nil {
		t.Fatal("compileBody() silently dropped an unavailable static value")
	}
	body, err := compileBody(`{{settings.voice | default: ""}}`, tokenestimate.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if body.Static == nil || *body.Static != "" {
		t.Fatalf("empty default body = %#v", body)
	}
	dynamic, err := compileBody(`{{model | default: ""}}`, tokenestimate.Context{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(dynamic)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Body
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dynamic, decoded) || decoded.Render(nil) != "" {
		t.Fatalf("empty runtime default round trip: encoded=%s decoded=%#v", encoded, decoded)
	}
}

func TestBuildRejectsInvalidStaticSlot(t *testing.T) {
	pkg := loadResolved(t, "invalid-slot", `  - id: static
    target: system
    text: Static.
`)
	input := inputFor(t, pkg, 1)
	input.Plans[0].Rules[0].Placements[0].StaticSlot = "unknown-slot"
	if _, err := Build(Request{StyleVersion: "1.0.0", Packages: []PackageInput{input}}); err == nil {
		t.Fatal("Build() accepted a static slot outside the index format")
	}
}

func TestSortStaticPreservesMultipleEntriesForSameRule(t *testing.T) {
	order := Order{Priority: 10, PackageSeq: 1, RuleIndex: 0}
	result := Result{Index: Index{Static: map[string][]StaticEntry{
		"codex": {
			{Rule: "pkg/rule", Slot: "z-slot", Source: "STYLE.yaml", BodySHA256: "z"},
			{Rule: "pkg/rule", Slot: "a-slot", Source: "STYLE.yaml", BodySHA256: "a"},
		},
	}}, StaticBodies: []StaticBody{
		{Harness: "codex", Rule: "pkg/rule", Slot: "z-slot", Order: order},
		{Harness: "codex", Rule: "pkg/rule", Slot: "a-slot", Order: order},
	}}
	sortResult(&result)
	entries := result.Index.Static["codex"]
	if len(entries) != 2 || entries[0].Slot != "a-slot" || entries[1].Slot != "z-slot" {
		t.Fatalf("static entries were lost or unstable: %#v", entries)
	}
}

func TestEncodeIsStableAndDoesNotEscapeHTML(t *testing.T) {
	value := "<style>"
	index := Index{Format: Format, StyleVersion: "1", Packages: []PackageInfo{}, Static: map[string][]StaticEntry{}, Harnesses: map[string]map[string][]Entry{
		"codex": {plan.PromptSubmitted: {{Rule: "pkg/rule", Order: Order{Priority: 1, PackageSeq: 1, RuleIndex: 0}, Action: "inject", When: structWhen(), Repeat: "session", Needs: []string{}, Body: Body{Static: &value}, Estimate: structEstimate()}}},
	}}
	first, err := Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode(index)
	if err != nil || !bytes.Equal(first, second) || !bytes.Contains(first, []byte("<style>")) || bytes.Contains(first, []byte("\\u003c")) {
		t.Fatalf("encode not stable or HTML-escaped: %s (%v)", first, err)
	}
}

func structWhen() when.Rule           { return when.Rule{Agent: []string{"main"}} }
func structEstimate() tokens.Estimate { return tokens.Estimate{Tokens: 1, Chars: 7} }
