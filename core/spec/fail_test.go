package spec

import "testing"

func TestM3A14FailValidation(t *testing.T) {
	for _, tc := range []struct{ name, defaults, rule, code string }{
		{"closed inject", "", "{id: r, target: tool, tool: shell, at: before, fail: closed, text: ok}", ""},
		{"closed warn", "", "{id: r, target: event, event: prompt.submitted, action: warn, fail: closed, text: ok}", ""},
		{"default closed", "defaults: {fail: closed}\n", "{id: r, target: event, event: tool.pre, text: ok}", ""},
		{"open system", "", "{id: r, target: system, fail: open, text: ok}", ""},
		{"invalid", "", "{id: r, target: tool, tool: shell, fail: invalid, text: ok}", "invalid-schema"},
		{"empty", "", "{id: r, target: event, event: tool.pre, fail: '', text: ok}", "invalid-schema"},
		{"bool", "", "{id: r, target: event, event: tool.pre, fail: true, text: ok}", "invalid-schema"},
		{"system", "", "{id: r, target: system, fail: closed, text: ok}", "fail-closed-target"},
		{"after", "", "{id: r, target: tool, tool: shell, at: after, fail: closed, text: ok}", "fail-closed-target"},
		{"post event", "", "{id: r, target: event, event: tool.post, fail: closed, text: ok}", "fail-closed-target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("guard", tc.defaults, "  - "+tc.rule+"\n"), nil)
			if tc.code != "" {
				if !hasDiagnostic(ds, tc.code, SeverityError) {
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
