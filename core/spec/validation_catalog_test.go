package spec

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func catalogPackage(t *testing.T, format, metadata, condition string) *Package {
	t.Helper()
	root := t.TempDir()
	header := "spec: style/v0.3\nname: catalog-fixture\nversion: 1.0.0\ndescription: Catalog fixture\n" + metadata
	when := "when: {harness: {name: " + condition + "}}\n"
	source := root
	switch format {
	case "directory":
		writeFile(t, root, "STYLE.yaml", header+"rules:\n  - id: r\n    target: system\n    text: body\n    "+when)
	case "from":
		writeFile(t, root, "STYLE.yaml", header+"rules:\n  - from: rules/r.md\n    "+when)
		// The override replaces this unregistered name as a whole.
		writeFile(t, root, "rules/r.md", "---\ntarget: system\nwhen: {harness: {name: replaced-harness}}\n---\nbody\n")
	case "single":
		writeFile(t, root, "fixture.style.md", "---\n"+header+"target: system\n"+when+"---\nbody\n")
		source = filepath.Join(root, "fixture.style.md")
	default:
		t.Fatalf("unknown catalog fixture format %q", format)
	}
	pkg, err := Load(source)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestM3A0CatalogControlsAllOrdinaryNames(t *testing.T) {
	for _, set := range []struct {
		name  string
		names []string
	}{
		{"two", []string{"codex", "claude-code"}},
		{"fake-only", []string{"fixture-harness"}},
		{"removed-codex", []string{"claude-code"}},
		{"empty", nil},
	} {
		for _, format := range []string{"directory", "from", "single"} {
			for _, name := range []string{"codex", "claude-code", "fixture-harness", "claude", "CODEX", " codex "} {
				for _, list := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%q/list=%v", set.name, format, name, list), func(t *testing.T) {
						condition := fmt.Sprintf("%q", name)
						if list {
							condition = "[" + condition + "]"
						}
						metadata := fmt.Sprintf("reads: {harness: [name]}\nrequires: {harness: {%q: '>=1.0.0'}}\nrecommends: {%q: {effort: high}}\n", name, name)
						pkg := catalogPackage(t, format, metadata, condition)
						if got := Validate(pkg); len(got) != 0 {
							t.Fatalf("structure must not guess names: %+v", got)
						}
						known := false
						for _, registered := range set.names {
							known = known || registered == name
						}
						var want []Diagnostic
						if !known {
							packageSource := "STYLE.yaml"
							if pkg.SingleFile {
								packageSource = pkg.Rules[0].Source
							}
							want = []Diagnostic{
								{Code: "invalid-schema", Path: packageSource, Severity: SeverityError, Message: fmt.Sprintf("unknown harness %q", name)},
								{Code: "invalid-schema", Path: packageSource, Severity: SeverityError, Message: fmt.Sprintf("unknown recommends harness %q", name)},
								{Code: "invalid-schema", Path: pkg.Rules[0].Source, Severity: SeverityError, Message: fmt.Sprintf("unknown harness name %q", name)},
							}
						}
						catalog := ValidationCatalog{Harnesses: set.names}
						if got := ValidateWithCatalog(pkg, catalog); !reflect.DeepEqual(got, want) {
							t.Fatalf("complete diagnostics = %+v, want %+v", got, want)
						}
						filename := ""
						if pkg.SingleFile {
							filename = "fixture.style.md"
						}
						snapshot, err := LoadSnapshot(pkg.Root, pkg.Files, filename)
						if err != nil {
							t.Fatal(err)
						}
						if got := ValidateWithCatalog(snapshot, catalog); !reflect.DeepEqual(got, want) {
							t.Fatalf("snapshot diagnostics = %+v, want %+v", got, want)
						}
					})
				}
			}
		}
	}
}

func TestM3A0CatalogDiagnosticCompatibility(t *testing.T) {
	metadata := "requires: {harness: {codex: invalid, mystery: false}}\nrecommends: {mystery: false}\n"
	pkg := catalogPackage(t, "directory", metadata, "[codex, mystery]")
	want := []Diagnostic{
		{Code: "invalid-schema", Path: "STYLE.yaml", Severity: SeverityError, Message: "requires.harness.codex must be a semantic version constraint"},
		{Code: "invalid-schema", Path: "STYLE.yaml", Severity: SeverityError, Message: `unknown harness "mystery"`},
		{Code: "invalid-schema", Path: "STYLE.yaml", Severity: SeverityError, Message: `unknown recommends harness "mystery"`},
		{Code: "invalid-schema", Path: "STYLE.yaml", Severity: SeverityError, Message: "recommends.mystery must be a mapping"},
		{Code: "invalid-schema", Path: pkg.Rules[0].Source, Severity: SeverityError, Message: `unknown harness name "mystery"`},
		{Code: "reads-missing", Path: pkg.Rules[0].Source, Severity: SeverityError, Message: "harness.name is used but not declared in reads.harness"},
	}
	for range 3 {
		if got := ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: []string{"codex", "claude-code"}}); !reflect.DeepEqual(got, want) {
			t.Fatalf("diagnostic facts/order = %+v, want %+v", got, want)
		}
	}
	for _, tc := range []struct{ name, metadata, condition, message string }{
		{"requires type", "requires: {harness: false}\nreads: {harness: [name]}\n", "codex", "requires.harness must be a mapping"},
		{"recommends type", "recommends: false\nreads: {harness: [name]}\n", "codex", "recommends must be a mapping"},
		{"condition type", "reads: {harness: [name]}\n", "[codex, 3]", "when.harness.name must be a string or list of strings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := catalogPackage(t, "directory", tc.metadata, tc.condition)
			got := ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: []string{"codex"}})
			if !reflect.DeepEqual(got, Validate(pkg)) || len(got) != 1 || got[0].Message != tc.message || got[0].Code != "invalid-schema" || got[0].Severity != SeverityError {
				t.Fatalf("structural diagnostics changed: %+v", got)
			}
		})
	}
	wantNil := []Diagnostic{{Code: "invalid-package", Message: "package is nil", Severity: SeverityError}}
	if got := ValidateWithCatalog(nil, ValidationCatalog{}); !reflect.DeepEqual(got, wantNil) || !reflect.DeepEqual(Validate(nil), wantNil) {
		t.Fatalf("nil diagnostics = %+v", got)
	}
}

func TestM3A0NativeUsesNameSupersetAndCapabilitySubset(t *testing.T) {
	surface := fakeNativeHarness{events: map[string]bool{"FixtureEvent": true}}
	for _, format := range []string{"directory", "single"} {
		for _, tc := range []struct {
			name, harness, message string
			catalog                ValidationCatalog
		}{
			{"native fake", "fixture-harness", "", ValidationCatalog{Harnesses: []string{"fixture-harness"}, Native: NativeCatalog{"fixture-harness": surface}}},
			{"ordinary only", "fixture-harness", `harness "fixture-harness" does not support native hooks`, ValidationCatalog{Harnesses: []string{"fixture-harness"}}},
			{"nil capability", "fixture-harness", `harness "fixture-harness" does not support native hooks`, ValidationCatalog{Harnesses: []string{"fixture-harness"}, Native: NativeCatalog{"fixture-harness": nil}}},
			{"extra native key", "codex", `unknown field "codex": not a STYLE field or registered harness (registered: fixture-harness)`, ValidationCatalog{Harnesses: []string{"fixture-harness"}, Native: NativeCatalog{"codex": surface}}},
			{"zero", "codex", `unknown field "codex": not a STYLE field or registered harness (registered: )`, ValidationCatalog{}},
			{"stable names", "missing", `unknown field "missing": not a STYLE field or registered harness (registered: a, z)`, ValidationCatalog{Harnesses: []string{"z", "a", "z"}}},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				fields := "target: native\n" + tc.harness + ": {event: FixtureEvent, matcher: '^ok$', command: 'printf never-run'}\n"
				pkg := loadNativeRule(t, format, fields, "")
				got := ValidateWithCatalog(pkg, tc.catalog)
				if tc.message == "" {
					if len(got) != 0 {
						t.Fatalf("native fake rejected: %+v", got)
					}
					return
				}
				want := []Diagnostic{{Code: "native-field", Path: pkg.Rules[0].Source, Severity: SeverityError, Message: tc.message}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("native membership = %+v, want %+v", got, want)
				}
			})
		}
		for _, fields := range []string{
			"target: native\ncodex: {event: UnknownEvent, command: echo}\n",
			"target: native\ncodex: {event: Stop, matcher: x, command: echo}\n",
			"target: native\ncodex: {event: PreToolUse, matcher: '(', command: echo}\n",
			"target: native\nclaude-code: {event: PreToolUse, matcher: '*', command: echo}\n",
			"target: native\nunknown: {event: Stop, command: echo}\n",
		} {
			pkg := loadNativeRule(t, format, fields, "")
			legacy := append(Validate(pkg), ValidateNative(pkg, testNativeCatalog)...)
			got := ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: []string{"codex", "claude-code"}, Native: testNativeCatalog})
			if !reflect.DeepEqual(got, legacy) {
				t.Fatalf("native compatibility: %+v != %+v", got, legacy)
			}
		}
	}
}

func TestM3A0ZeroCatalogAndConcurrentCatalogs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("generic", "", "  - {id: r, target: system, text: body}\n"))
	generic, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateWithCatalog(generic, ValidationCatalog{}); len(got) != 0 {
		t.Fatalf("generic package must not require a registered harness: %+v", got)
	}
	pkg := catalogPackage(t, "from", "reads: {harness: [name]}\n", "fixture-harness")
	var workers sync.WaitGroup
	for index := range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			catalog := ValidationCatalog{}
			if index%2 == 0 {
				catalog.Harnesses = []string{"fixture-harness"}
			}
			for range 10 {
				got := ValidateWithCatalog(pkg, catalog)
				if index%2 == 0 && len(got) != 0 || index%2 != 0 && (len(got) != 1 || !strings.Contains(got[0].Message, "unknown harness name")) {
					t.Errorf("catalogs interfered: %+v", got)
					return
				}
			}
		}()
	}
	workers.Wait()
}
