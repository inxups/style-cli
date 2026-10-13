package spec

import "testing"

func TestParseYAMLMapRequiresOneCompleteDocument(t *testing.T) {
	for _, input := range []string{
		"name: first\n---\nname: second\n",
		"name: first\n---\nname: [unterminated\n",
		"name: first\n---\n",
		"name: first\n...\ntrailing text\n",
		"[name, first]\n",
		"plain scalar\n",
		"# no mapping\n",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := parseYAMLMap([]byte(input), "STYLE.yaml")
			if ErrorCode(err) != "invalid-yaml" {
				t.Fatalf("parseYAMLMap() error = %v, want invalid-yaml", err)
			}
			if parsed, ok := err.(*ParseError); !ok || parsed.Path != "STYLE.yaml" {
				t.Fatalf("error source = %#v, want STYLE.yaml", err)
			}
		})
	}
	for _, input := range []string{"name: first\n# trailing comment\n", "---\nname: first\n...\n# comment\n", "{}\n"} {
		if _, err := parseYAMLMap([]byte(input), "STYLE.yaml"); err != nil {
			t.Errorf("valid mapping %q rejected: %v", input, err)
		}
	}
}

func TestLoadRejectsTrailingYAMLDocument(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("single-document", "", "  - id: base\n    target: system\n    text: body\n")+"---\nname: ignored\n")
	if _, err := Load(root); ErrorCode(err) != "invalid-yaml" {
		t.Fatalf("Load() error = %v, want invalid-yaml", err)
	}
}
