package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestM3A10EventValidationAndReads(t *testing.T) {
	valid := "reads: {event: [prompt]}\n"
	rules := "  - {id: r, target: event, event: prompt.submitted, text: '{{event.prompt}}'}\n"
	for _, tc := range []struct{ name, fields, rules, code string }{
		{"valid", valid, rules, ""},
		{"missing", "", rules, "reads-missing"},
		{"wrong type", "reads: {event: true}\n", rules, "invalid-schema"},
		{"unknown read", "reads: {event: [cwd]}\n", rules, "invalid-schema"},
		{"duplicate read", "reads: {event: [prompt, prompt]}\n", rules, "invalid-schema"},
		{"unused", "reads: {event: [prompt, tool.name]}\n", rules, "reads-unused"},
		{"bare", valid, strings.ReplaceAll(rules, "event.prompt", "prompt"), "event-var-in-template"},
		{"unknown", valid, strings.ReplaceAll(rules, "event.prompt", "event.cwd"), "undeclared-var"},
		{"wrong point", valid, strings.ReplaceAll(rules, "event: prompt.submitted", "event: tool.pre"), "event-var-unavailable"},
		{"warn implicit", valid, strings.ReplaceAll(rules, "target: event", "action: warn, target: event"), "event-warn-model-text"},
		{"warn explicit", valid, strings.ReplaceAll(rules, "target: event", "action: warn, model_text: 'Review {{event.prompt}}', target: event"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("event-fixture", tc.fields, tc.rules), nil)
			if tc.code != "" {
				if !hasDiagnostic(ds, tc.code, SeverityError) && !hasDiagnostic(ds, tc.code, SeverityWarning) {
					t.Fatal(ds)
				}
			} else {
				for _, d := range ds {
					if d.Severity == SeverityError {
						t.Fatal(ds)
					}
				}
			}
		})
	}
}
func TestM3A10EventReadsFromUnionAndFieldAvailability(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("events-union", "reads: {event: [prompt]}\n", "  - from: rules/base.md\n    reads: {event: [tool.output]}\n"))
	writeFile(t, root, "rules/base.md", "---\ntarget: event\nevent: tool.pre\nreads: {event: [tool.input.command]}\n---\n{{event.tool.input.command}}\n")
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range Validate(pkg) {
		if d.Severity == SeverityError {
			t.Fatal(d)
		}
	}
	merged, err := Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stringValues(merged.Reads["event"]), []string{"prompt", "tool.input.command", "tool.output"}) {
		t.Fatal(merged.Reads)
	}
	// A single-file frontmatter declaration follows the same validation path.
	file := filepath.Join(root, "single.style.md")
	if err := os.WriteFile(file, []byte("---\nspec: style/v0.3\nname: single\nversion: 1.0.0\ndescription: event fixture\ntarget: event\nevent: tool.post\nreads: {event: [tool.output]}\n---\n{{event.tool.output}}"), 0600); err != nil {
		t.Fatal(err)
	}
	single, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range Validate(single) {
		if d.Severity == SeverityError {
			t.Fatal(d)
		}
	}
}
