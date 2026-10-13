package spec

import (
	"reflect"
	"strings"
	"testing"
)

func TestMergeAppliesPrecedenceAndUnionsReads(t *testing.T) {
	root := t.TempDir()
	manifest := strings.Join([]string{
		"spec: style/v0.3",
		"name: merge-test",
		"version: 0.1.0",
		"description: Merge test",
		"priority: 30",
		"budget:",
		"  per_rule: 240",
		"reads:",
		"  model: true",
		"  harness: [name, cwd]",
		"defaults:",
		"  action: warn",
		"  at: before",
		"  repeat: turn",
		"  reinject_on_compact: false",
		"  sources: [resume]",
		"  fallback: skip",
		"  priority: 20",
		"  max_tokens: 120",
		"rules:",
		"  - from: rules/merge.md",
		"    action: inject",
		"    priority: 90",
		"    when:",
		"      model: codex-*",
		"      thinking_effort: [high]",
		"    reads:",
		"      harness: [turn_id]",
		"  - id: disabled",
		"    target: tool",
		"    text: Do not include.",
		"    enabled: false",
		"  - id: inline",
		"    target: system",
		"    text: Shares package reads.",
		"    reads:",
		"      harness: [version]",
	}, "\n")
	writeFile(t, root, "STYLE.yaml", manifest)
	rule := strings.Join([]string{
		"---",
		"target: tool",
		"action: block",
		"priority: 60",
		"max_tokens: 80",
		"fallback: error",
		"reinject_on_compact: true",
		"sources: [startup]",
		"when:",
		"  model: claude-*",
		"  thinking_effort: [low]",
		"  provider: old-provider",
		"reads:",
		"  thinking_effort: true",
		"  harness: [cwd, agent]",
		"---",
		"Merged rule.",
	}, "\n")
	writeFile(t, root, "rules/merge.md", rule)

	pkg, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	merged, err := Merge(pkg)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	if len(merged.Rules) != 2 {
		t.Fatalf("merged rule count = %d, want disabled rule filtered", len(merged.Rules))
	}
	fields := merged.Rules[0].Effective
	wants := map[string]any{
		"id":                  "merge",
		"target":              "tool",
		"action":              "inject",
		"at":                  "before",
		"repeat":              "turn",
		"reinject_on_compact": true,
		"fallback":            "error",
		"priority":            90,
		"max_tokens":          80,
		"enabled":             true,
	}
	for key, want := range wants {
		if got := fields[key]; got != want {
			t.Errorf("effective %s = %#v, want %#v", key, got, want)
		}
	}
	if got, want := stringValues(fields["sources"]), []string{"startup"}; !reflect.DeepEqual(got, want) {
		t.Errorf("effective sources = %#v, want %#v", got, want)
	}
	reads := asStringMap(fields["reads"])
	if reads["model"] != true || reads["thinking_effort"] != true {
		t.Errorf("effective reads = %#v, expected model and thinking_effort", reads)
	}
	if got, want := stringValues(reads["harness"]), []string{"name", "cwd", "agent", "turn_id", "version"}; !reflect.DeepEqual(got, want) {
		t.Errorf("effective reads.harness = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(merged.Reads, reads) || !reflect.DeepEqual(merged.Rules[1].Effective["reads"], reads) {
		t.Errorf("all rule declarations must be unioned at package level: %#v", merged.Reads)
	}
	when := asStringMap(fields["when"])
	if when["model"] != "codex-*" || !reflect.DeepEqual(stringValues(when["thinking_effort"]), []string{"high"}) {
		t.Errorf("from override should replace the complete when value: %#v", when)
	}
	if _, exists := when["provider"]; exists {
		t.Errorf("from override retained a frontmatter condition: %#v", when)
	}
	if pkg.Rules[1].Fields["enabled"] != false {
		t.Fatal("Merge mutated the source package")
	}
}

func TestMergeStandardDefaultsDependOnTargetAndAction(t *testing.T) {
	root := t.TempDir()
	manifest := strings.Join([]string{
		"spec: style/v0.3",
		"name: defaults-test",
		"version: 0.1.0",
		"description: Defaults test",
		"priority: 30",
		"budget:",
		"  per_rule: 240",
		"defaults:",
		"  priority: 20",
		"  max_tokens: 120",
		"rules:",
		"  - id: system-rule",
		"    target: system",
		"    text: System.",
		"  - id: warning",
		"    target: tool",
		"    action: warn",
		"    text: Warning.",
		"  - id: block",
		"    target: tool",
		"    action: block",
		"    repeat: session",
		"    text: Block.",
	}, "\n")
	writeFile(t, root, "STYLE.yaml", manifest)
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.Rules[0].Effective["priority"]; got != 20 {
		t.Errorf("priority default = %#v, want defaults.priority 20 over package priority 30", got)
	}
	if got := merged.Rules[0].Effective["max_tokens"]; got != 120 {
		t.Errorf("max_tokens default = %#v, want defaults.max_tokens 120 over budget.per_rule 240", got)
	}
	if got := merged.Rules[0].Effective["repeat"]; got != "session" {
		t.Errorf("system repeat = %#v, want session", got)
	}
	if got := merged.Rules[0].Effective["reinject_on_compact"]; got != true {
		t.Errorf("system reinject default = %#v, want true", got)
	}
	if got := merged.Rules[0].Effective["mode"]; got != "append" {
		t.Errorf("system mode default = %#v, want append", got)
	}
	if got, want := stringValues(merged.Rules[0].Effective["sources"]), []string{"startup", "clear", "fork"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sources default = %#v, want %#v", got, want)
	}
	if got := merged.Rules[0].Effective["fallback"]; got != "degrade" {
		t.Errorf("fallback default = %#v, want degrade", got)
	}
	if got := merged.Rules[1].Effective["repeat"]; got != "every" {
		t.Errorf("warn repeat = %#v, want every", got)
	}
	if _, ok := merged.Rules[2].Effective["repeat"]; ok {
		t.Errorf("block repeat should be ignored, got %#v", merged.Rules[2].Effective["repeat"])
	}
	if got := merged.Rules[2].Effective["at"]; got != "before" {
		t.Errorf("block at default = %#v, want before", got)
	}
	if merged.Rules[2].Fields["repeat"] != "session" {
		t.Fatal("Merge discarded the raw block repeat declaration")
	}
}

func TestMergeMinimalPackageDefaults(t *testing.T) {
	merged, err := Merge(&Package{Rules: []Rule{{Fields: map[string]any{"id": "simple", "target": "system"}}}})
	if err != nil {
		t.Fatal(err)
	}
	fields := merged.Rules[0].Effective
	for key, want := range map[string]any{"priority": 50, "max_tokens": 400, "action": "inject", "at": "after", "enabled": true} {
		if got := fields[key]; got != want {
			t.Errorf("default %s = %#v, want %#v", key, got, want)
		}
	}
}
