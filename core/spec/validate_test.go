package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateRequiredCodes(t *testing.T) {
	cases := []struct {
		name, packageFields, rules, want string
		files                            map[string]string
	}{
		{"dup id", "", "  - id: same\n    target: system\n    text: first\n  - id: same\n    target: system\n    text: second\n", "dup-id", nil},
		{"missing inline id", "", "  - target: system\n    text: body\n", "inline-missing-id", nil},
		{"file and text", "", "  - id: base\n    target: system\n    file: body.md\n    text: inline\n", "file-and-text", map[string]string{"body.md": "body\n"}},
		{"defaults field", "defaults:\n  target: system\n", "  - id: base\n    target: system\n    text: body\n", "defaults-field", nil},
		{"undeclared variable", "", "  - id: base\n    target: system\n    text: '{{vars.unknown}}'\n", "undeclared-var", nil},
		{"reads missing", "", "  - id: base\n    target: system\n    text: '{{model}}'\n", "reads-missing", nil},
		{"event variable", "", "  - id: base\n    target: system\n    text: '{{prompt}}'\n", "event-var-in-template", nil},
		{"sensitive reserved", "settings:\n  - name: token\n    sensitive: true\n", "  - id: base\n    target: system\n    text: body\n", "sensitive-reserved", nil},
		{"condition wrong event", "", "  - id: base\n    target: event\n    event: session.start\n    text: body\n    when:\n      conditions:\n        - field: tool.name\n          op: equals\n          pattern: Bash\n", "condition-field-wrong-event", nil},
		{"bad regex", "", "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.input.command\n          op: regex\n          pattern: '['\n", "bad-regex", nil},
		{"bad shell regex", "", "  - id: base\n    target: tool\n    tool: shell\n    text: body\n    when:\n      conditions:\n        - field: tool.input.command\n          op: shell_regex\n          pattern: '['\n", "bad-regex", nil},
		{"bad effort", "", "  - id: base\n    target: system\n    text: body\n    when:\n      thinking_effort: extreme\n", "bad-effort-expr", nil},
		{"block target", "", "  - id: base\n    target: system\n    action: block\n    text: reason\n", "action-target", nil},
		{"block after", "", "  - id: base\n    target: tool\n    tool: shell\n    action: block\n    at: after\n    text: reason\n", "block-after", nil},
		{"replace target", "", "  - id: base\n    target: tool\n    tool: shell\n    mode: replace\n    text: body\n", "replace-non-system", nil},
		{"bad id", "", "  - id: ../escape\n    target: system\n    text: body\n", "bad-id", nil},
		{"marker in body", "", "  - id: base\n    target: system\n    text: '<!-- style: owned -->'\n", "marker-in-body", nil},
		{"bad name", "", "  - id: base\n    target: system\n    text: body\n", "bad-name", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "valid-package"
			if tc.name == "bad name" {
				name = "Invalid_Name"
			}
			diagnostics := loadAndValidate(t, testManifest(name, tc.packageFields, tc.rules), tc.files)
			if !hasDiagnostic(diagnostics, tc.want, SeverityError) {
				t.Fatalf("Validate() diagnostics = %#v, want %s", diagnostics, tc.want)
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == tc.want && ErrorCode(diagnostic) != tc.want {
					t.Fatalf("ErrorCode(diagnostic) = %q, want %q", ErrorCode(diagnostic), tc.want)
				}
			}
		})
	}
}

func TestValidateImportedRulesAndWarnings(t *testing.T) {
	cases := []struct {
		name, ruleEntry, file, want string
	}{
		{"from plus inline", "  - from: rules/base.md\n    text: override\n", "---\ntarget: system\n---\nbody\n", "from-and-inline"},
		{"from protected target", "  - from: rules/base.md\n    target: event\n", "---\ntarget: system\n---\nbody\n", "override-target"},
		{"rule file package field", "  - from: rules/base.md\n", "---\nname: wrong\ntarget: system\n---\nbody\n", "rule-file-pkg-field"},
		{"empty body", "  - from: rules/base.md\n", "---\ntarget: system\n---\n\n", "empty-body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := testManifest("valid-package", "", tc.ruleEntry)
			diagnostics := loadAndValidate(t, manifest, map[string]string{"rules/base.md": tc.file})
			if !hasDiagnostic(diagnostics, tc.want, SeverityError) {
				t.Fatalf("Validate() diagnostics = %#v, want %s", diagnostics, tc.want)
			}
		})
	}

	manifest := testManifest("warnings", "reads:\n  model: true\n", "  - id: base\n    target: system\n    text: body\n")
	diagnostics := loadAndValidate(t, manifest, map[string]string{"rules/unused.md": "---\ntarget: system\n---\nnot loaded\n"})
	if !hasDiagnostic(diagnostics, "undeclared-rule-file", SeverityWarning) || !hasDiagnostic(diagnostics, "reads-unused", SeverityWarning) {
		t.Fatalf("Validate() diagnostics = %#v, want undeclared-rule-file and reads-unused warnings", diagnostics)
	}
}

func TestValidateAcceptsDeclaredTemplateVariables(t *testing.T) {
	packageFields := "reads:\n  model: true\nvars:\n  lang: zh-CN\nsettings:\n  - name: detail\n    default: concise\n"
	rules := "  - id: base\n    target: system\n    text: '{{vars.lang}} {{settings.detail}} {{model | default: \"unknown\"}} {{rule.id}}'\n    when:\n      model: gpt-*\n"
	diagnostics := loadAndValidate(t, testManifest("valid-package", packageFields, rules), nil)
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			t.Fatalf("Validate() unexpected error: %#v", diagnostic)
		}
	}
}

func TestValidateUsesRuleFileReadsDeclarations(t *testing.T) {
	manifest := testManifest("rule-reads", "", "  - from: rules/base.md\n")
	diagnostics := loadAndValidate(t, manifest, map[string]string{
		"rules/base.md": "---\ntarget: system\nreads:\n  model: true\n---\nModel: {{model}}\n",
	})
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			t.Fatalf("Validate() unexpected error: %#v", diagnostic)
		}
	}
}

func TestValidateSingleFilePackage(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "single.style.md")
	data := []byte("---\nspec: style/v0.3\nname: single\nversion: 1.0.0\ndescription: One rule\ntarget: system\n---\n\nKeep it concise.\n")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := Load(file)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, diagnostic := range Validate(pkg) {
		if diagnostic.Severity == SeverityError {
			t.Fatalf("Validate() unexpected error: %#v", diagnostic)
		}
	}
}

func testManifest(name, packageFields, rules string) string {
	return fmt.Sprintf("spec: style/v0.3\nname: %s\nversion: 1.2.3\ndescription: A test package\n%srules:\n%s", name, packageFields, rules)
}

func loadAndValidate(t *testing.T, manifest string, files map[string]string) []Diagnostic {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", manifest)
	for name, contents := range files {
		writeFile(t, root, name, contents)
	}
	pkg, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return Validate(pkg)
}

func hasDiagnostic(diagnostics []Diagnostic, code, severity string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Severity == severity {
			return true
		}
	}
	return false
}

func TestValidateStableDiagnosticOrder(t *testing.T) {
	manifest := testManifest("valid-package", "", "  - id: base\n    target: system\n    text: '{{vars.missing}} {{model}}'\n")
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", manifest)
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	first, second := Validate(pkg), Validate(pkg)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("diagnostic order changed: %#v != %#v", first, second)
	}
}
