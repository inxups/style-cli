package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirectoryExpandsDeclaredRulesInManifestOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", `spec: style/v0.3
name: review-helper
version: 1.2.3
description: Review style
rules:
  - id: base
    target: system
    file: prompts/base.md
  - id: inline-text
    target: skill
    text: Use the repository review style.
  - from: rules/direct.md
  - from: rules/review/**/*.md
    priority: 17
`)
	writeFile(t, root, "prompts/base.md", "Be concise.\n")
	writeFile(t, root, "rules/direct.md", `---
target: event
action: inject
---
Direct from rule.
`)
	writeFile(t, root, "rules/review/z.md", `---
target: tool
action: warn
---
Warn on z.
`)
	writeFile(t, root, "rules/review/a.md", `---
target: tool
action: block
---
Block on a.
`)
	writeFile(t, root, "rules/review/deep/nested.md", `---
target: mcp
---
Nested rule.
`)
	writeFile(t, root, "rules/not-declared.md", `---
target: skill
---
Not implicitly loaded.
`)

	pkg, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if pkg.SingleFile {
		t.Fatal("directory package marked as single-file")
	}
	if got := pkg.Metadata["name"]; got != "review-helper" {
		t.Fatalf("metadata name = %#v, want review-helper", got)
	}
	if got := len(pkg.Rules); got != 6 {
		t.Fatalf("rule count = %d, want 6", got)
	}
	wantSources := []string{"STYLE.yaml#base", "STYLE.yaml#inline-text", "rules/direct.md", "rules/review/a.md", "rules/review/deep/nested.md", "rules/review/z.md"}
	wantIDs := []string{"base", "inline-text", "direct", "review/a", "review/deep/nested", "review/z"}
	for i, rule := range pkg.Rules {
		if rule.Index != i {
			t.Errorf("rule[%d].Index = %d", i, rule.Index)
		}
		if rule.Source != wantSources[i] {
			t.Errorf("rule[%d].Source = %q, want %q", i, rule.Source, wantSources[i])
		}
		if got := rule.Fields["id"]; got != wantIDs[i] {
			t.Errorf("rule[%d].id = %#v, want %q", i, got, wantIDs[i])
		}
	}
	if got := strings.TrimSpace(pkg.Rules[0].Body); got != "Be concise." {
		t.Errorf("inline file body = %q", got)
	}
	if got := strings.TrimSpace(pkg.Rules[1].Body); got != "Use the repository review style." {
		t.Errorf("inline text body = %q", got)
	}
	if got := strings.TrimSpace(pkg.Rules[2].Body); got != "Direct from rule." {
		t.Errorf("direct from body = %q", got)
	}
	if got := strings.TrimSpace(pkg.Rules[3].Body); got != "Block on a." {
		t.Errorf("globbed frontmatter body = %q", got)
	}
	if got := pkg.Rules[3].Overrides["priority"]; got != 17 {
		t.Errorf("from override priority = %#v, want 17", got)
	}
	if _, ok := pkg.Files["rules/not-declared.md"]; ok {
		t.Error("unreferenced rule file was loaded implicitly")
	}
}

func TestLoadSingleFilePackageDefaultsRuleIDToPackageName(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "my-style.style.md")
	content := "---\nspec: style/v0.3\nname: my-style\nversion: 0.1.0\ndescription: Single rule\npriority: 31\ntarget: system\naction: inject\n---\n\nKeep the review focused.  \n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := Load(file)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !pkg.SingleFile {
		t.Fatal("single-file package not marked as such")
	}
	if pkg.Metadata["name"] != "my-style" {
		t.Fatalf("metadata name = %#v", pkg.Metadata["name"])
	}
	if len(pkg.Rules) != 1 {
		t.Fatalf("rule count = %d, want 1", len(pkg.Rules))
	}
	rule := pkg.Rules[0]
	if rule.Fields["id"] != "my-style" {
		t.Errorf("default rule id = %#v, want my-style", rule.Fields["id"])
	}
	if rule.Fields["priority"] != 31 || pkg.Metadata["priority"] != 31 {
		t.Errorf("priority should be package and rule metadata: rule=%#v package=%#v", rule.Fields["priority"], pkg.Metadata["priority"])
	}
	if rule.Body != "Keep the review focused." {
		t.Errorf("body = %q", rule.Body)
	}
}

func TestLoadReportsMalformedFrontmatterAndEscapingFromPath(t *testing.T) {
	t.Run("malformed frontmatter", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "broken.style.md", "---\nname: [unterminated\n---\nbody\n")
		_, err := Load(filepath.Join(root, "broken.style.md"))
		if got := ErrorCode(err); got != "invalid-yaml" {
			t.Fatalf("ErrorCode() = %q, want invalid-yaml; err=%v", got, err)
		}
	})

	t.Run("escaping from path", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "STYLE.yaml", `spec: style/v0.3
name: path-test
version: 0.1.0
description: Test
rules:
  - from: ../outside.md
`)
		_, err := Load(root)
		if got := ErrorCode(err); got != "path-escape" {
			t.Fatalf("ErrorCode() = %q, want path-escape; err=%v", got, err)
		}
	})

	t.Run("body file with frontmatter", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "STYLE.yaml", `spec: style/v0.3
name: frontmatter-test
version: 0.1.0
description: Test
rules:
  - id: rule
    target: system
    file: prompts/body.md
`)
		writeFile(t, root, "prompts/body.md", "---\nthis-is-not-body: true\n---\nBody\n")
		_, err := Load(root)
		if got := ErrorCode(err); got != "body-has-frontmatter" {
			t.Fatalf("ErrorCode() = %q, want body-has-frontmatter; err=%v", got, err)
		}
	})

	t.Run("file reference into rules", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "STYLE.yaml", `spec: style/v0.3
name: file-into-rules
version: 0.1.0
description: Test
rules:
  - id: rule
    target: system
    file: rules/body.md
`)
		writeFile(t, root, "rules/body.md", "Body\n")
		_, err := Load(root)
		if got := ErrorCode(err); got != "file-into-rules" {
			t.Fatalf("ErrorCode() = %q, want file-into-rules; err=%v", got, err)
		}
	})
}

func TestMatchGlobSupportsRecursiveZeroOrMoreSegments(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"rules/**/*.md", "rules/a.md", true},
		{"rules/**/*.md", "rules/nested/a.md", true},
		{"rules/**/*.md", "rules/nested/deep/a.md", true},
		{"rules/*.md", "rules/nested/a.md", false},
		{"rules/*.md", "rules/a.txt", false},
	}
	for _, tc := range cases {
		got, err := matchGlob(tc.pattern, tc.name)
		if err != nil {
			t.Errorf("matchGlob(%q, %q) error = %v", tc.pattern, tc.name, err)
		} else if got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestCleanRelativePathRejectsEscapes(t *testing.T) {
	for _, input := range []string{"../secret.md", "rules/../../secret.md", "/tmp/a.md", `rules\a.md`, "C:/temp/a.md"} {
		if _, err := cleanRelativePath(input); ErrorCode(err) != "path-escape" {
			t.Errorf("cleanRelativePath(%q) error = %v, want path-escape", input, err)
		}
	}
}

func TestLoadRejectsLinkedOversizedAndInvalidTextFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "STYLE.yaml", "spec: style/v0.3\nname: link-test\nversion: 0.1.0\ndescription: Test\nrules: []\n")
		target := filepath.Join(root, "outside.md")
		if err := os.WriteFile(target, []byte("body"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "linked.md")); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}
		if _, err := readPackageFile(root, "linked.md", map[string][]byte{}); ErrorCode(err) != "symlink" {
			t.Fatalf("readPackageFile symlink error = %v, want symlink", err)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "STYLE.yaml", "spec: style/v0.3\nname: large-test\nversion: 0.1.0\ndescription: Test\nrules:\n  - id: rule\n    target: system\n    file: prompts/body.md\n")
		writeFile(t, root, "prompts/body.md", strings.Repeat("x", MaxFileSize+1))
		if _, err := Load(root); ErrorCode(err) != "file-too-large" {
			t.Fatalf("Load oversized file error = %v, want file-too-large", err)
		}
	})

	t.Run("invalid UTF-8", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte{0xff, 0xfe}, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readPackageFile(root, "STYLE.yaml", map[string][]byte{}); ErrorCode(err) != "bad-encoding" {
			t.Fatalf("readPackageFile invalid UTF-8 error = %v, want bad-encoding", err)
		}
	})

	t.Run("NUL byte", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "STYLE.yaml"), []byte{'x', 0, 'y'}, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readPackageFile(root, "STYLE.yaml", map[string][]byte{}); ErrorCode(err) != "bad-encoding" {
			t.Fatalf("readPackageFile NUL error = %v, want bad-encoding", err)
		}
	})
}

func writeFile(t *testing.T, root, name, contents string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
