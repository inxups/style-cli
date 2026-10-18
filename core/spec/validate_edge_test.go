package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDefaultsEnumEvenWhenRulesOverrideIt(t *testing.T) {
	manifest := testManifest("defaults-enum", "defaults:\n  action: invalid\n", "  - id: base\n    target: system\n    action: inject\n    text: body\n")
	diagnostics := loadAndValidate(t, manifest, nil)
	if !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
		t.Fatalf("Validate() diagnostics = %#v, want invalid defaults.action error", diagnostics)
	}
}

func TestValidateRejectsRemovedReadsFromOverride(t *testing.T) {
	manifest := testManifest("override-reads", "", "  - from: rules/base.md\n    reads:\n      model: true\n")
	diagnostics := loadAndValidate(t, manifest, map[string]string{
		"rules/base.md": "---\ntarget: system\n---\nStatic body.\n",
	})
	if !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
		t.Fatalf("Validate() diagnostics = %#v, want invalid-schema for removed reads override", diagnostics)
	}
}

func TestValidateRejectsEmptyAndMalformedTargetSelectors(t *testing.T) {
	cases := []struct {
		name string
		rule string
	}{
		{"empty canonical tool", "  - id: base\n    target: tool\n    tool: ''\n    text: body\n"},
		{"empty native name list", "  - id: base\n    target: tool\n    names: []\n    text: body\n"},
		{"blank native name in list", "  - id: base\n    target: tool\n    names: [Edit, ' ']\n    text: body\n"},
		{"empty skill selector", "  - id: base\n    target: skill\n    skill: ['  ']\n    text: body\n"},
		{"mcp selector missing slash", "  - id: base\n    target: mcp\n    mcp: server\n    text: body\n"},
		{"mcp selector has extra slash", "  - id: base\n    target: mcp\n    mcp: server/tool/extra\n    text: body\n"},
		{"mcp selector has empty component", "  - id: base\n    target: mcp\n    mcp: server/\n    text: body\n"},
		{"mcp selector has surrounding whitespace", "  - id: base\n    target: mcp\n    mcp: ' server/tool'\n    text: body\n"},
		{"mcp component has surrounding whitespace", "  - id: base\n    target: mcp\n    mcp: 'server /tool'\n    text: body\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := loadAndValidate(t, testManifest("bad-selector", "", tc.rule), nil)
			if !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
				t.Fatalf("Validate() diagnostics = %#v, want invalid-schema", diagnostics)
			}
		})
	}
}

func TestValidateAcceptsWellFormedTargetSelectors(t *testing.T) {
	cases := []struct {
		name string
		rule string
	}{
		{"canonical and native tools", "  - id: base\n    target: tool\n    tool: [file.edit, file.write]\n    names: [Edit, Bash]\n    text: body\n"},
		{"mcp wildcards", "  - id: base\n    target: mcp\n    mcp: ['server/*', '*/search']\n    text: body\n"},
		{"skill name", "  - id: base\n    target: skill\n    skill: code-review\n    text: body\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := loadAndValidate(t, testManifest("good-selector", "", tc.rule), nil)
			if hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
				t.Fatalf("Validate() diagnostics = %#v, want selectors accepted", diagnostics)
			}
		})
	}
}

func TestValidateFromOverrideTemplateAndSelector(t *testing.T) {
	manifest := testManifest("from-override", "settings:\n  - name: known\n    default: value\n", "  - from: rules/base.md\n    tool: file.write\n    model_text: '{{settings.missing}}'\n")
	diagnostics := loadAndValidate(t, manifest, map[string]string{
		"rules/base.md": "---\ntarget: tool\naction: warn\n---\nBase body.\n",
	})
	if !hasDiagnostic(diagnostics, "undeclared-var", SeverityError) {
		t.Fatalf("Validate() diagnostics = %#v, want undeclared-var in from model_text", diagnostics)
	}
	if hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
		t.Fatalf("Validate() rejected target selector override: %#v", diagnostics)
	}
}

func TestValidateSingleFileRejectsRuleList(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "single.style.md")
	data := []byte("---\nspec: style/v0.3\nname: single\nversion: 1.0.0\ndescription: One rule\ntarget: system\nrules: []\n---\nBody.\n")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := Load(file)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !hasDiagnostic(Validate(pkg), "invalid-schema", SeverityError) {
		t.Fatal("Validate() did not reject a rules list in a single-file package")
	}
}
