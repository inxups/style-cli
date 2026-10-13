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

func TestValidateUnusedReadsFromFromOverride(t *testing.T) {
	manifest := testManifest("override-reads", "", "  - from: rules/base.md\n    reads:\n      model: true\n")
	diagnostics := loadAndValidate(t, manifest, map[string]string{
		"rules/base.md": "---\ntarget: system\n---\nStatic body.\n",
	})
	if !hasDiagnostic(diagnostics, "reads-unused", SeverityWarning) {
		t.Fatalf("Validate() diagnostics = %#v, want reads-unused for from override", diagnostics)
	}
}

func TestValidateFromOverrideTemplateAndSelector(t *testing.T) {
	manifest := testManifest("from-override", "vars:\n  known: value\n", "  - from: rules/base.md\n    tool: file.write\n    model_text: '{{vars.missing}}'\n")
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
