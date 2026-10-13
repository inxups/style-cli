package spec

import "testing"

func TestValidateModelGlobSyntax(t *testing.T) {
	for _, pattern := range []string{"[", "gpt-[", "**/["} {
		rule := "  - id: base\n    target: system\n    text: body\n    when:\n      model: '" + pattern + "'\n"
		diagnostics := loadAndValidate(t, testManifest("bad-model-glob", "reads: {model: true}\n", rule), nil)
		if !hasDiagnostic(diagnostics, "bad-model-glob", SeverityError) {
			t.Errorf("model glob %q diagnostics = %#v, want bad-model-glob", pattern, diagnostics)
		}
	}
	for _, pattern := range []string{"gpt-*", "**/claude-*", "claude-opus"} {
		rule := "  - id: base\n    target: system\n    text: body\n    when:\n      model: '" + pattern + "'\n"
		diagnostics := loadAndValidate(t, testManifest("good-model-glob", "reads: {model: true}\n", rule), nil)
		if hasDiagnostic(diagnostics, "bad-model-glob", SeverityError) {
			t.Errorf("valid model glob %q rejected: %#v", pattern, diagnostics)
		}
	}
}
