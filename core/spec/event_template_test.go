package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM3A10EventValidation(t *testing.T) {
	rules := "  - {id: r, target: event, event: prompt.submitted, text: '{{event.prompt}}'}\n"
	for _, tc := range []struct{ name, rules, code string }{
		{"valid", rules, ""},
		{"bare", strings.ReplaceAll(rules, "event.prompt", "prompt"), "event-var-in-template"},
		{"unknown", strings.ReplaceAll(rules, "event.prompt", "event.cwd"), "undeclared-var"},
		{"wrong point", strings.ReplaceAll(rules, "event: prompt.submitted", "event: tool.pre"), "event-var-unavailable"},
		{"warn implicit", strings.ReplaceAll(rules, "target: event", "action: warn, target: event"), "event-warn-model-text"},
		{"warn explicit", strings.ReplaceAll(rules, "target: event", "action: warn, model_text: 'Review {{event.prompt}}', target: event"), ""},
		{"output path", "  - {id: r, target: event, event: tool.post, text: '{{event.tool.output.stdout}}'}\n", ""},
		{"bare output path", "  - {id: r, target: event, event: tool.post, text: '{{tool.output.stdout}}'}\n", "event-var-in-template"},
		{"output path before tool", "  - {id: r, target: event, event: tool.pre, text: '{{event.tool.output.stdout}}'}\n", "event-var-unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("event-fixture", "", tc.rules), nil)
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
func TestM3A10EventFieldAvailability(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("events", "", "  - from: rules/base.md\n"))
	writeFile(t, root, "rules/base.md", "---\ntarget: event\nevent: tool.pre\n---\n{{event.tool.input.command}}\n")
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range Validate(pkg) {
		if d.Severity == SeverityError {
			t.Fatal(d)
		}
	}
	// A single-file frontmatter rule follows the same validation path.
	file := filepath.Join(root, "single.style.md")
	if err := os.WriteFile(file, []byte("---\nspec: style/v0.3\nname: single\nversion: 1.0.0\ndescription: event fixture\ntarget: event\nevent: tool.post\n---\n{{event.tool.output}}"), 0600); err != nil {
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
