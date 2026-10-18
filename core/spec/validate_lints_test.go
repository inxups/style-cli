package spec

import (
	"strings"
	"testing"
)

func TestValidateBlockReasonTokenLint(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       bool
	}{
		{name: "at limit", text: strings.Repeat("a", 160)},
		{name: "over limit", text: strings.Repeat("a", 161), want: true},
		{name: "CJK", text: strings.Repeat("界", 41), want: true},
		{name: "runtime placeholder", text: "{{harness.cwd}}", want: false},
		{name: "large runtime placeholder", text: "{{harness.cwd}} x" + strings.Repeat("a", 100), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := "  - id: base\n    target: tool\n    tool: shell\n    action: block\n    text: " + quoteYAML(tc.text) + "\n"
			diagnostics := loadAndValidate(t, testManifest("lint-case", "", rules), nil)
			if got := hasDiagnostic(diagnostics, "T005", SeverityWarning); got != tc.want {
				t.Fatalf("T005 warning = %t, want %t; diagnostics = %#v", got, tc.want, diagnostics)
			}
		})
	}

	longValue := strings.Repeat("a", 161)
	manifest := testManifest("lint-static-variable", "settings:\n  - name: long\n    default: "+quoteYAML(longValue)+"\n", "  - id: base\n    target: tool\n    tool: shell\n    action: block\n    text: '{{settings.long}}'\n")
	if !hasDiagnostic(loadAndValidate(t, manifest, nil), "T005", SeverityWarning) {
		t.Fatal("T005 warning missing for a long static setting value")
	}
}

func TestValidateT006AndT007Warnings(t *testing.T) {
	longLivedVariable := testManifest("lint-t006", "reads:\n  harness: [session_id]\n", "  - id: base\n    target: system\n    text: '{{harness.session_id}}'\n")
	if !hasDiagnostic(loadAndValidate(t, longLivedVariable, nil), "T006", SeverityWarning) {
		t.Fatal("T006 warning missing for harness.session_id")
	}

	repeatedInject := testManifest("lint-t007", "", "  - id: base\n    target: system\n    action: inject\n    repeat: every\n    text: body\n")
	if !hasDiagnostic(loadAndValidate(t, repeatedInject, nil), "T007", SeverityWarning) {
		t.Fatal("T007 warning missing for repeated inject without reason")
	}
}

func TestValidateT001UsesEffectiveLimitAndModelFacingWarnText(t *testing.T) {
	tooLong := testManifest("lint-t001-error", "budget:\n  per_rule: 1\n  on: error\n", "  - id: long\n    target: system\n    action: inject\n    text: abcde\n")
	if diagnostics := loadAndValidate(t, tooLong, nil); !hasDiagnostic(diagnostics, "T001", SeverityError) {
		t.Fatalf("T001 should follow budget.on: error; diagnostics = %#v", diagnostics)
	}

	shortWarnModelText := testManifest("lint-t001-warn", "budget:\n  per_rule: 2\n", "  - id: warning\n    target: tool\n    tool: shell\n    action: warn\n    text: 'OK. "+strings.Repeat("x", 80)+"'\n    model_text: hi\n")
	if diagnostics := loadAndValidate(t, shortWarnModelText, nil); hasDiagnostic(diagnostics, "T001", SeverityWarning) || hasDiagnostic(diagnostics, "T001", SeverityError) {
		t.Fatalf("T001 should estimate warn.model_text, not its user message; diagnostics = %#v", diagnostics)
	}

	perRuleOverride := testManifest("lint-t001-override", "budget:\n  per_rule: 20\ndefaults:\n  max_tokens: 4\n", "  - id: override\n    target: system\n    max_tokens: 2\n    text: abcdefghijk\n")
	if diagnostics := loadAndValidate(t, perRuleOverride, nil); !hasDiagnostic(diagnostics, "T001", SeverityWarning) {
		t.Fatalf("rule max_tokens should override package/default limits; diagnostics = %#v", diagnostics)
	}
}

func TestEstimateTokensMatchesOfflineFormula(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"abcd", 1},
		{"你好", 2},
		{"!", 1},
		{"a!", 1},
		{"abcde", 2},
	} {
		if got := estimateTokens(tc.text); got != tc.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func quoteYAML(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, "'", "''"), "\n", "\\n") + "'"
}
