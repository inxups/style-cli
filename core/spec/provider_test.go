package spec

import "testing"

func TestM3A11ProviderValidation(t *testing.T) {
	for _, tc := range []struct{ name, rule, code string }{
		{"valid", "  - {id: r, target: system, when: {provider: [openai, custom.ID]}, text: '{{provider}}'}\n", ""},
		{"wrong type", "  - {id: r, target: system, when: {provider: {id: openai}}, text: ok}\n", "invalid-schema"},
		{"URL", "  - {id: r, target: system, when: {provider: 'https://private.invalid'}, text: ok}\n", "bad-provider-id"},
		{"explicit unknown", "  - {id: r, target: system, when: {provider: unknown}, text: ok}\n", ""},
		{"unconditional warning", "  - {id: r, target: system, text: '{{provider}}'}\n", "var-no-default"},
		{"default", "  - {id: r, target: system, text: '{{provider | default: \"missing\"}}'}\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("provider", "", tc.rule), nil)
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
	writeFile(t, root, "STYLE.yaml", testManifest("provider-rule-file", "", "  - from: rules/r.md\n"))
	writeFile(t, root, "rules/r.md", "---\ntarget: system\n---\n{{model | default: \"m\"}} {{provider | default: \"p\"}}\n")
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range Validate(pkg) {
		if d.Severity == SeverityError {
			t.Fatal(d)
		}
	}
}
