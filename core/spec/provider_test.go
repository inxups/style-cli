package spec

import (
	"reflect"
	"testing"
)

func TestM3A11ProviderReadsAndValidation(t *testing.T) {
	for _, tc := range []struct{ name, reads, rule, code string }{
		{"valid", "reads: {provider: true}\n", "  - {id: r, target: system, when: {provider: [openai, custom.ID]}, text: '{{provider}}'}\n", ""},
		{"missing", "", "  - {id: r, target: system, when: {provider: openai}, text: ok}\n", "reads-missing"},
		{"template missing", "", "  - {id: r, target: system, text: '{{provider}}'}\n", "reads-missing"},
		{"false", "reads: {provider: false}\n", "  - {id: r, target: system, text: '{{provider}}'}\n", "invalid-schema"},
		{"wrong type", "reads: {provider: [openai]}\n", "  - {id: r, target: system, text: ok}\n", "invalid-schema"},
		{"unused", "reads: {provider: true}\n", "  - {id: r, target: system, text: ok}\n", "reads-unused"},
		{"URL", "reads: {provider: true}\n", "  - {id: r, target: system, when: {provider: 'https://private.invalid'}, text: ok}\n", "bad-provider-id"},
		{"explicit unknown", "reads: {provider: true}\n", "  - {id: r, target: system, when: {provider: unknown}, text: ok}\n", ""},
		{"unconditional warning", "reads: {provider: true}\n", "  - {id: r, target: system, text: '{{provider}}'}\n", "var-no-default"},
		{"default", "reads: {provider: true}\n", "  - {id: r, target: system, text: '{{provider | default: \"missing\"}}'}\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("provider", tc.reads, tc.rule), nil)
			if tc.code != "" {
				if !hasDiagnostic(ds, tc.code, SeverityError) && !hasDiagnostic(ds, tc.code, SeverityWarning) {
					t.Fatal(ds)
				}
			} else {
				for _, d := range ds {
					if d.Severity == SeverityError || d.Code == "provider-unimplemented" || d.Code == "var-no-default" {
						t.Fatal(ds)
					}
				}
			}
		})
	}
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("provider-union", "reads: {model: true}\n", "  - from: rules/r.md\n    reads: {provider: true}\n"))
	writeFile(t, root, "rules/r.md", "---\ntarget: system\nreads: {provider: true}\n---\n{{model}} {{provider}}\n")
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
	if err != nil || !reflect.DeepEqual(merged.Reads, map[string]any{"model": true, "provider": true}) {
		t.Fatal(merged.Reads, err)
	}
}
