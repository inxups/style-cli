package spec

import "testing"

func TestValidateUsesTemplateParserForEscapesAndQuotedDefaults(t *testing.T) {
	manifest := testManifest("template-parser", "reads:\n  model: true\n", "  - id: body\n    target: system\n    text: 'literal \\{{name}} {{model | default: \"x }} y\"}}'\n")
	diagnostics := loadAndValidate(t, manifest, nil)
	if hasDiagnostic(diagnostics, "invalid-template", SeverityError) || hasDiagnostic(diagnostics, "event-var-in-template", SeverityError) {
		t.Fatalf("valid escaped/default template diagnostics = %#v", diagnostics)
	}
}

func TestValidateReportsMalformedTemplate(t *testing.T) {
	manifest := testManifest("bad-template", "", "  - id: body\n    target: system\n    text: '{{model | default: unquoted}}'\n")
	diagnostics := loadAndValidate(t, manifest, nil)
	if !hasDiagnostic(diagnostics, "invalid-template", SeverityError) {
		t.Fatalf("Validate() diagnostics = %#v, want invalid-template", diagnostics)
	}
}
