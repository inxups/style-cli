package spec

import "testing"

func TestM3A12UnknownValidationAndTemplateGuarantees(t *testing.T) {
	for _, tc := range []struct{ name, condition, text, code string }{
		{"model", "model: [gpt-*, unknown]", "{{model}}", "var-no-default"},
		{"provider", "provider: [openai, unknown]", "{{provider}}", "var-no-default"},
		{"effort", "thinking_effort: [high, unknown]", "{{thinking_effort}}", "var-no-default"},
		{"raw effort", "thinking_effort: unknown", "{{thinking_effort_raw}}", "var-no-default"},
		{"mode", "harness: {permission_mode: [default, unknown]}", "{{harness.permission_mode}}", "var-no-default"},
		{"default", "model: unknown", "{{model | default: \"missing\"}}", ""},
		{"uppercase model", "model: UNKNOWN", "ok", ""},
		{"effort comparison", "thinking_effort: '>=unknown'", "ok", "bad-effort-expr"},
		{"approx is boolean", "thinking_effort_approx: unknown", "ok", "invalid-schema"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("unknown", "", "  - {id: r, target: system, when: {"+tc.condition+"}, text: '"+tc.text+"'}\n"), nil)
			if tc.code != "" {
				if !hasDiagnostic(ds, tc.code, SeverityError) && !hasDiagnostic(ds, tc.code, SeverityWarning) {
					t.Fatal(ds)
				}
			} else {
				for _, d := range ds {
					if d.Severity == SeverityError || d.Code == "var-no-default" {
						t.Fatal(ds)
					}
				}
			}
		})
	}
	pkg := catalogPackage(t, "directory", "", "unknown")
	if !hasDiagnostic(ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: []string{"codex", "claude-code"}}), "invalid-schema", SeverityError) {
		t.Fatal("harness registry membership bypassed")
	}
}
