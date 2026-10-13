package spec

import "testing"

func TestLoadRejectsNonMarkdownRuleSources(t *testing.T) {
	for _, pattern := range []string{"rules/test.txt", "rules/*.txt", "rules/*", "prompts/test.md"} {
		t.Run(pattern, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "STYLE.yaml", testManifest("source-case", "", "  - from: "+pattern+"\n"))
			for _, name := range []string{"rules/test.txt", "rules/valid.md", "prompts/test.md"} {
				writeFile(t, root, name, "---\ntarget: system\n---\nbody\n")
			}
			if _, err := Load(root); ErrorCode(err) != "invalid-schema" {
				t.Fatalf("Load() error = %v, want invalid-schema", err)
			}
		})
	}
}

func TestValidateRejectsNonMarkdownRuleSource(t *testing.T) {
	for _, source := range []string{"rules/test.txt", "prompts/test.md"} {
		root := t.TempDir()
		writeFile(t, root, "STYLE.yaml", testManifest("valid-source", "", "  - from: rules/test.md\n"))
		writeFile(t, root, "rules/test.md", "---\ntarget: system\n---\nbody\n")
		pkg, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		pkg.Rules[0].Source = source
		if diagnostics := Validate(pkg); !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
			t.Fatalf("Validate(%q) diagnostics = %#v, want invalid-schema", source, diagnostics)
		}
	}
}
