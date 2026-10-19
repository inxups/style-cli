package spec

import (
	"strings"
	"testing"
)

func TestValidateConditionSelectorsAndOptionalFields(t *testing.T) {
	cases := []struct {
		name, rule, want, severity string
	}{
		{
			name: "tool added for unrelated canonical tool",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.added\n          op: contains\n          pattern: TODO\n",
			want: "condition-field-wrong-event", severity: SeverityError,
		},
		{
			name: "tool added for file write",
			rule: "  - id: base\n    target: tool\n    tool: file.write\n    text: body\n    when:\n      conditions:\n        - field: tool.added\n          op: contains\n          pattern: TODO\n",
		},
		{
			name: "tool added for native Edit selector",
			rule: "  - id: base\n    target: tool\n    names: Edit\n    at: before\n    text: body\n    when:\n      conditions:\n        - field: tool.added\n          op: contains\n          pattern: TODO\n",
		},
		{
			name: "tool added for native name list",
			rule: "  - id: base\n    target: tool\n    names: [Edit, Bash]\n    at: after\n    text: body\n    when:\n      conditions:\n        - field: tool.added\n          op: contains\n          pattern: TODO\n",
		},
		{
			name: "negative operation on optional field",
			rule: "  - id: base\n    target: tool\n    tool: file.edit\n    text: body\n    when:\n      conditions:\n        - field: tool.added\n          op: not_contains\n          pattern: TODO\n",
			want: "negated-optional-field", severity: SeverityWarning,
		},
		{
			name: "empty not_contains pattern",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.input.command\n          op: not_contains\n          pattern: \"\"\n",
			want: "empty-pattern", severity: SeverityError,
		},
		{
			name: "empty contains pattern",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.input.command\n          op: contains\n          pattern: \"\"\n",
			want: "empty-pattern", severity: SeverityError,
		},
		{
			name: "empty equals pattern stays valid",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.input.command\n          op: equals\n          pattern: \"\"\n",
		},
		{
			name: "whole tool input",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.input\n          op: contains\n          pattern: git push --force\n",
			want: "whole-tool-input", severity: SeverityWarning,
		},
		{
			name: "whole tool output",
			rule: "  - id: base\n    target: tool\n    tool: file.edit\n    at: after\n    text: body\n    when:\n      conditions:\n        - field: tool.output\n          op: contains\n          pattern: FAIL\n",
			want: "whole-tool-output", severity: SeverityWarning,
		},
		{
			name: "tool output path after tool",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    at: after\n    text: body\n    when:\n      conditions:\n        - field: tool.output.stdout\n          op: contains\n          pattern: FAIL\n",
		},
		{
			name: "tool output path before tool",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    at: before\n    text: body\n    when:\n      conditions:\n        - field: tool.output.stdout\n          op: contains\n          pattern: FAIL\n",
			want: "condition-field-wrong-event", severity: SeverityError,
		},
		{
			name: "negative operation on tool output path",
			rule: "  - id: base\n    target: tool\n    tool: shell\n    at: after\n    text: body\n    when:\n      conditions:\n        - field: tool.output.stdout\n          op: not_contains\n          pattern: FAIL\n",
			want: "negated-optional-field", severity: SeverityWarning,
		},
		{
			name: "tool added on mcp rule",
			rule: "  - id: base\n    target: mcp\n    mcp: server/tool\n    text: body\n    when:\n      conditions:\n        - field: tool.added\n          op: contains\n          pattern: TODO\n",
			want: "condition-field-wrong-event", severity: SeverityError,
		},
		{
			name: "tool output on skill before",
			rule: "  - id: base\n    target: skill\n    skill: review\n    at: before\n    text: body\n    when:\n      conditions:\n        - field: tool.output\n          op: contains\n          pattern: marker\n",
			want: "condition-field-wrong-event", severity: SeverityError,
		},
		{
			name: "tool output on skill after",
			rule: "  - id: base\n    target: skill\n    skill: review\n    at: after\n    text: body\n    when:\n      conditions:\n        - field: tool.output\n          op: contains\n          pattern: marker\n",
		},
		{
			name: "prompt remains available on skill before explicit event",
			rule: "  - id: base\n    target: skill\n    skill: review\n    at: before\n    text: body\n    when:\n      conditions:\n        - field: prompt\n          op: contains\n          pattern: review\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := loadAndValidate(t, testManifest("condition-case", "", tc.rule), nil)
			if tc.want == "" {
				for _, diagnostic := range diagnostics {
					if diagnostic.Severity == SeverityError {
						t.Fatalf("Validate() unexpected error: %#v", diagnostic)
					}
				}
				return
			}
			if !hasDiagnostic(diagnostics, tc.want, tc.severity) {
				t.Fatalf("Validate() diagnostics = %#v, want %s/%s", diagnostics, tc.want, tc.severity)
			}
		})
	}
}

func TestValidateWholeToolOutputSkipsShellOnlyRules(t *testing.T) {
	shell := "  - id: base\n    target: tool\n    tool: shell\n    at: after\n    text: body\n    when:\n      conditions:\n        - field: tool.output\n          op: contains\n          pattern: FAIL\n"
	if diagnostics := loadAndValidate(t, testManifest("condition-case", "", shell), nil); hasDiagnostic(diagnostics, "whole-tool-output", SeverityWarning) {
		t.Fatalf("shell-only rule should not warn: %#v", diagnostics)
	}
	native := strings.Replace(shell, "tool: shell\n", "names: Bash\n", 1)
	if diagnostics := loadAndValidate(t, testManifest("condition-case", "", native), nil); !hasDiagnostic(diagnostics, "whole-tool-output", SeverityWarning) {
		t.Fatalf("native names rule should still warn: %#v", diagnostics)
	}
}

func TestValidateHarnessNameValues(t *testing.T) {
	pkg := catalogPackage(t, "directory", "", "unknown")
	diagnostics := ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: []string{"codex", "claude-code"}})
	if !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
		t.Fatalf("Validate() diagnostics = %#v, want invalid harness name error", diagnostics)
	}
}
