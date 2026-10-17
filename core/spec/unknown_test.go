package spec

import "testing"

func TestM3A12UnknownValidationReadsAndTemplateGuarantees(t *testing.T) {
	for _, tc := range []struct{ name, reads, condition, text, code string }{
		{"model", "model: true", "model: [gpt-*, unknown]", "{{model}}", "var-no-default"},
		{"provider", "provider: true", "provider: [openai, unknown]", "{{provider}}", "var-no-default"},
		{"effort", "thinking_effort: true", "thinking_effort: [high, unknown]", "{{thinking_effort}}", "var-no-default"},
		{"raw effort", "thinking_effort: true", "thinking_effort: unknown", "{{thinking_effort_raw}}", "var-no-default"},
		{"mode", "harness: [permission_mode]", "harness: {permission_mode: [default, unknown]}", "{{harness.permission_mode}}", "var-no-default"},
		{"default", "model: true", "model: unknown", "{{model | default: \"missing\"}}", ""},
		{"uppercase model", "model: true", "model: UNKNOWN", "ok", ""},
		{"effort comparison", "thinking_effort: true", "thinking_effort: '>=unknown'", "ok", "bad-effort-expr"},
		{"approx is boolean", "thinking_effort: true", "thinking_effort_approx: unknown", "ok", "invalid-schema"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("unknown", "reads: {"+tc.reads+"}\n", "  - {id: r, target: system, when: {"+tc.condition+"}, text: '"+tc.text+"'}\n"), nil)
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
	missing := loadAndValidate(t, testManifest("unknown", "", "  - {id: r, target: system, when: {model: unknown}, text: ok}\n"), nil)
	if !hasDiagnostic(missing, "reads-missing", SeverityError) {
		t.Fatal(missing)
	}
	pkg := catalogPackage(t, "directory", "reads: {harness: [name]}\n", "unknown")
	if !hasDiagnostic(ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: []string{"codex", "claude-code"}}), "invalid-schema", SeverityError) {
		t.Fatal("harness registry membership bypassed")
	}
}
