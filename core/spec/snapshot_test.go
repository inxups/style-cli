package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSnapshotIsDetachedAndNeverConsultsDisk(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"STYLE.yaml":           []byte("spec: style/v0.3\nname: memory\nversion: 0.1.0\ndescription: memory\nrules:\n  - from: rules/**/*.md\n"),
		"rules/nested/test.md": []byte("---\ntarget: tool\ntool: shell\n---\nApproved snapshot.\n"),
	}
	pkg, err := LoadSnapshot(root, files, "")
	if err != nil {
		t.Fatal(err)
	}
	files["rules/nested/test.md"][0] = 'X'
	files["STYLE.yaml"] = []byte("not the approved source")
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "unexpected.md"), []byte("disk only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pkg.Rules[0].Body != "Approved snapshot." || !strings.HasPrefix(string(pkg.Files["rules/nested/test.md"]), "---") {
		t.Fatalf("snapshot aliased input: %#v", pkg)
	}
	if diagnostics := Validate(pkg); len(diagnostics) != 0 {
		t.Fatalf("validation consulted disk: %+v", diagnostics)
	}
	if pkg.Rules[0].Fields["id"] != "nested/test" {
		t.Fatalf("glob/id = %#v", pkg.Rules[0])
	}
}

func TestLoadSnapshotKeepsUnusedRuleDiagnostics(t *testing.T) {
	files := map[string][]byte{
		"STYLE.yaml":      []byte("spec: style/v0.3\nname: memory\nversion: 0.1.0\ndescription: memory\nrules:\n  - id: base\n    target: system\n    text: body\n"),
		"rules/unused.md": []byte("not loaded"),
	}
	pkg, err := LoadSnapshot("", files, "")
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := Validate(pkg); len(diagnostics) != 1 || diagnostics[0].Code != "undeclared-rule-file" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestLoadSnapshotMatchesDiskSingleFileAndRejectsInvalidInputs(t *testing.T) {
	data := []byte("---\nspec: style/v0.3\nname: single\nversion: 0.1.0\ndescription: single\ntarget: system\n---\nA single rule.\n")
	root := t.TempDir()
	path := filepath.Join(root, "original.style.md")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	disk, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	memory, err := LoadSnapshot(root, map[string][]byte{"original.style.md": data}, "original.style.md")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(disk.Manifest, memory.Manifest) || !reflect.DeepEqual(disk.Rules, memory.Rules) || !reflect.DeepEqual(Validate(disk), Validate(memory)) {
		t.Fatal("single-file loaders disagree")
	}
	for _, tc := range []struct {
		name, code string
		files      map[string][]byte
		single     string
	}{
		{"escape", "path-escape", map[string][]byte{"../STYLE.yaml": data}, ""},
		{"noncanonical", "path-escape", map[string][]byte{"a/./b": data}, ""},
		{"encoding", "bad-encoding", map[string][]byte{"STYLE.yaml": {0xff}}, ""},
		{"large", "file-too-large", map[string][]byte{"STYLE.yaml": make([]byte, MaxFileSize+1)}, ""},
		{"missing", "file-not-found", map[string][]byte{}, ""},
		{"multi", "invalid-package", map[string][]byte{"a.style.md": data, "b.style.md": data}, "a.style.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadSnapshot(root, tc.files, tc.single); ErrorCode(err) != tc.code {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
		})
	}
}
