package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type invalidFixture struct {
	Manifest string            `yaml:"manifest"`
	Files    map[string]string `yaml:"files"`
	Generate struct {
		Kind   string `yaml:"kind"`
		Path   string `yaml:"path"`
		Target string `yaml:"target"`
		Size   int    `yaml:"size"`
	} `yaml:"generate"`
}

func TestA1ValidateExampleAndEveryInvalidFixture(t *testing.T) {
	root := repositoryRoot(t)
	service := testService(t)
	var stdout, stderr bytes.Buffer
	if code := RunWithService(context.Background(), service, root, []string{"validate", "example/terse-reviewer", "--json"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("example exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	// Keep the acceptance contract explicit so deleting a fixture cannot make
	// this suite green by silently reducing its scope.
	want := strings.Fields("dup-id from-and-inline inline-missing-id file-and-text file-into-rules body-has-frontmatter rule-file-pkg-field empty-body override-target defaults-field undeclared-var reads-missing event-var-in-template sensitive-reserved condition-field-wrong-event bad-regex bad-effort-expr action-target block-after replace-non-system bad-name bad-id path-escape symlink file-too-large bad-encoding marker-in-body native-mapping native-field native-event native-matcher native-command native-timeout")
	paths, err := filepath.Glob(filepath.Join(root, "testdata", "invalid", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(paths))
	for _, path := range paths {
		got = append(got, strings.TrimSuffix(filepath.Base(path), ".yaml"))
	}
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("invalid fixture inventory=%v, want=%v", got, want)
	}
	for _, path := range paths {
		code := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(code, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var fixture invalidFixture
			decoder := yaml.NewDecoder(bytes.NewReader(data))
			decoder.KnownFields(true)
			if err := decoder.Decode(&fixture); err != nil {
				t.Fatal(err)
			}
			packageDir := writeInstallPackage(t, t.TempDir(), fixture.Manifest)
			for name, value := range fixture.Files {
				writeSyncFile(t, filepath.Join(packageDir, name), value)
			}
			generated := filepath.Join(packageDir, fixture.Generate.Path)
			switch fixture.Generate.Kind {
			case "":
			case "symlink":
				if err := os.Symlink(fixture.Generate.Target, generated); err != nil {
					t.Fatal(err)
				}
			case "large":
				writeSyncFile(t, generated, strings.Repeat("x", fixture.Generate.Size))
			case "invalid-utf8":
				writeSyncFile(t, generated, string([]byte{0xff, 0xfe}))
			default:
				t.Fatalf("unknown fixture generator %q", fixture.Generate.Kind)
			}
			for _, jsonOutput := range []bool{false, true} {
				args := []string{"validate", packageDir}
				if jsonOutput {
					args = append(args, "--json")
				}
				var out, errOut bytes.Buffer
				exit := RunWithService(context.Background(), service, root, args, &out, &errOut)
				if exit != ExitInvalid || errOut.Len() > 0 {
					t.Fatalf("exit=%d stdout=%s stderr=%s", exit, out.String(), errOut.String())
				}
				if jsonOutput {
					var diagnostics []Diagnostic
					if err := json.Unmarshal(out.Bytes(), &diagnostics); err != nil {
						t.Fatal(err)
					}
					found := false
					for _, diagnostic := range diagnostics {
						if diagnostic.Code == code && diagnostic.Severity == "error" {
							found = true
						}
					}
					if !found {
						t.Fatalf("expected error %s, got %s", code, out.String())
					}
				} else if !strings.Contains(out.String(), code+" ") && !strings.Contains(out.String(), code+":") {
					t.Fatalf("missing text diagnostic %s: %s", code, out.String())
				}
			}
		})
	}
}

func TestValidateRejectsRemovedVarsField(t *testing.T) {
	root := repositoryRoot(t)
	service := testService(t)
	packageDir := writeInstallPackage(t, t.TempDir(), "spec: style/v0.3\nname: removed-vars\nversion: 0.1.0\ndescription: removed vars\nvars:\n  lang: zh-CN\nrules:\n  - id: base\n    target: system\n    text: 'Use {{vars.lang}}.'\n")
	var out, errOut bytes.Buffer
	if exit := RunWithService(context.Background(), service, root, []string{"validate", packageDir, "--json"}, &out, &errOut); exit != ExitInvalid || errOut.Len() > 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, out.String(), errOut.String())
	}
	var diagnostics []Diagnostic
	if err := json.Unmarshal(out.Bytes(), &diagnostics); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			found[diagnostic.Code] = true
		}
	}
	if !found["invalid-schema"] || !found["undeclared-var"] {
		t.Fatalf("diagnostics = %s, want invalid-schema and undeclared-var", out.String())
	}
}
