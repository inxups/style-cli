package spec

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeRuleSchemaMatchesAcrossPackageFormats(t *testing.T) {
	cases := []struct {
		name, fields string
		wantError    bool
	}{
		{"codex", "target: native\ncodex: {}\n", false},
		{"claude", "target: native\nclaude-code: {}\n", false},
		{"both", "target: native\ncodex: {}\nclaude-code: {}\n", false},
		{"unknown native field", "target: native\ncodex: {}\nunknown: {}\n", true},
		{"non-native provider field", "target: system\ncodex: {}\n", true},
		{"non-mapping hook", "target: native\ncodex: []\n", true},
	}
	for _, tc := range cases {
		for _, format := range []string{"directory", "single-file"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				root := t.TempDir()
				source := root
				if format == "directory" {
					fields := strings.ReplaceAll(strings.TrimSuffix(tc.fields, "\n"), "\n", "\n    ")
					writeFile(t, root, "STYLE.yaml", testManifest("native-rule", "", "  - id: native-rule\n    "+fields+"\n    text: native instructions\n"))
				} else {
					writeFile(t, root, "native-rule.style.md", "---\nspec: style/v0.3\nname: native-rule\nversion: 1.0.0\ndescription: Native rule\n"+tc.fields+"---\nnative instructions\n")
					source = filepath.Join(root, "native-rule.style.md")
				}
				pkg, err := Load(source)
				if err != nil {
					t.Fatal(err)
				}
				diagnostics := Validate(pkg)
				if hasErrorDiagnostic(diagnostics) != tc.wantError {
					t.Fatalf("Validate() diagnostics = %#v, wantError=%v", diagnostics, tc.wantError)
				}
			})
		}
	}
}

func hasErrorDiagnostic(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			return true
		}
	}
	return false
}
