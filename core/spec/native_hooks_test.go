package spec

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const nativeHookHeader = "spec: style/v0.3\nname: hook-package\nversion: 1.0.0\ndescription: Hook package\n"

func TestNativeHookCollectionsLoadAndMergeWithoutRuleDefaults(t *testing.T) {
	root := t.TempDir()
	manifest := nativeHookHeader + `priority: 30
defaults: {action: block, priority: 5, fallback: error, repeat: every, max_tokens: 1}
native_hooks:
  - id: notify
    description: '{{ deliberately not a template'
    codex: {event: Stop, command: notify}
  - id: explicit
    priority: 10.5
    fallback: error
    codex: {event: Stop, command: notify}
  - id: disabled
    enabled: false
    codex: {event: Stop, command: notify}
`
	writeFile(t, root, "STYLE.yaml", manifest)
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if ds := validateWithNative(pkg, testNativeCatalog); len(ds) != 0 {
		t.Fatal(ds)
	}
	snapshot, err := LoadSnapshot(pkg.Root, pkg.Files, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pkg.NativeHooks, snapshot.NativeHooks) || len(pkg.Rules) != 0 || len(pkg.NativeHooks) != 3 {
		t.Fatal("disk and snapshot hook declarations differ")
	}
	resolved, err := Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Rules) != 0 || len(resolved.NativeHooks) != 2 {
		t.Fatal("hook enablement changed rule collection")
	}
	hook := resolved.NativeHooks[0]
	if hook.Effective["enabled"] != true || hook.Effective["priority"] != 30 || hook.Effective["fallback"] != "skip" || hook.Effective["action"] != nil || hook.Effective["max_tokens"] != nil {
		t.Fatalf("rule defaults leaked: %+v", hook.Effective)
	}
	if resolved.NativeHooks[1].Effective["priority"] != 10.5 || resolved.NativeHooks[1].Effective["fallback"] != "error" {
		t.Fatal("explicit hook fields lost")
	}
	hook.Fields["codex"].(map[string]any)["command"] = "mutated"
	if pkg.NativeHooks[0].Fields["codex"].(map[string]any)["command"] != "notify" {
		t.Fatal("merge mutated source")
	}
}

func TestNativeHookSchemaAndSharedIdentity(t *testing.T) {
	cases := []struct{ name, body, code string }{
		{"mixed", "rules: [{id: rule, target: system, text: Body.}]\nnative_hooks: [{id: hook, codex: {event: Stop, command: notify}}]\n", ""},
		{"hook only", "native_hooks: [{id: hook, codex: {event: Stop, command: notify}}]\n", ""},
		{"empty rules", "rules: []\nnative_hooks: [{id: hook, codex: {event: Stop, command: notify}}]\n", ""},
		{"empty description", "native_hooks: [{id: hook, description: '', codex: {event: Stop, command: notify}}]\n", ""},
		{"empty", "rules: []\nnative_hooks: []\n", "invalid-schema"},
		{"missing lists", "", "invalid-schema"},
		{"rules null", "rules: null\n", "invalid-package"},
		{"hooks null", "native_hooks: null\n", "invalid-package"},
		{"hooks map", "native_hooks: {}\n", "invalid-package"},
		{"scalar hook", "native_hooks: [true]\n", "invalid-package"},
		{"missing id", "native_hooks: [{codex: {event: Stop, command: notify}}]\n", "bad-id"},
		{"rule hook duplicate", "rules: [{id: same, target: system, text: Body.}]\nnative_hooks: [{id: same, codex: {event: Stop, command: notify}}]\n", "dup-id"},
		{"hook duplicate", "native_hooks: [{id: same, codex: {event: Stop, command: a}}, {id: same, codex: {event: Stop, command: b}}]\n", "dup-id"},
		{"disabled invalid", "native_hooks: [{id: hook, enabled: false, codex: {event: Stop}}]\n", "native-command"},
		{"nested non-string key", "native_hooks: [{id: hook, codex: {event: Stop, command: notify, nested: {1: value}}}]\n", "invalid-schema"},
	}
	for _, field := range []string{"target: native", "text: notes", "file: notes.md", "from: hooks/a.yaml", "when: {}", "action: warn", "fail: closed", "refresh: 1", "repeat: every", "fallback: degrade", "priority: .inf", "enabled: yes", "description: 3"} {
		cases = append(cases, struct{ name, body, code string }{field, "native_hooks: [{id: hook, " + field + ", codex: {event: Stop, command: notify}}]\n", "native-field"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := LoadSnapshot("fixture", map[string][]byte{"STYLE.yaml": []byte(nativeHookHeader + tc.body)}, "")
			if err != nil {
				if ErrorCode(err) != tc.code {
					t.Fatalf("%v want %s", err, tc.code)
				}
				return
			}
			ds := validateWithNative(pkg, testNativeCatalog)
			if tc.code == "" {
				if len(errorCodes(ds)) != 0 {
					t.Fatal(ds)
				}
			} else if !hasDiagnostic(ds, tc.code, SeverityError) {
				t.Fatalf("%+v want %s", ds, tc.code)
			}
		})
	}
}

func TestHarnessMappingCannotBeDeclaredOnOrdinaryRule(t *testing.T) {
	pkg, err := LoadSnapshot("fixture", map[string][]byte{"STYLE.yaml": []byte(nativeHookHeader + "rules: [{id: base, target: system, text: Body., codex: {event: Stop, command: notify}}]\n")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if ds := validateWithNative(pkg, testNativeCatalog); !hasDiagnostic(ds, "invalid-schema", SeverityError) {
		t.Fatal(ds)
	}
}

func TestRemovedNativeTargetRejectedAtEverySourceEntry(t *testing.T) {
	for _, format := range []string{"inline", "disabled", "from", "override", "single"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			single := ""
			files := map[string][]byte{}
			switch format {
			case "inline":
				files["STYLE.yaml"] = []byte(nativeHookHeader + "rules: [{id: old, target: native, codex: {event: Stop, command: notify}}]\n")
			case "disabled":
				files["STYLE.yaml"] = []byte(nativeHookHeader + "rules: [{id: old, target: native, enabled: false, codex: {event: Stop, command: notify}}]\n")
			case "from":
				files["STYLE.yaml"] = []byte(nativeHookHeader + "rules: [{from: rules/old.md}]\n")
				files["rules/old.md"] = []byte("---\ntarget: native\ncodex: {event: Stop, command: notify}\n---\n")
			case "override":
				files["STYLE.yaml"] = []byte(nativeHookHeader + "rules: [{from: rules/old.md, target: native}]\n")
				files["rules/old.md"] = []byte("---\ntarget: system\n---\nBody.\n")
			case "single":
				single = "old.style.md"
				files[single] = []byte("---\n" + nativeHookHeader + "target: native\ncodex: {event: Stop, command: notify}\n---\n")
			}
			for name, data := range files {
				writeFile(t, root, name, string(data))
			}
			source := root
			if single != "" {
				source = filepath.Join(root, single)
			}
			for _, load := range []func() (*Package, error){func() (*Package, error) { return Load(source) }, func() (*Package, error) { return LoadSnapshot(root, files, single) }} {
				if _, err := load(); ErrorCode(err) != NativeFormatRemoved || !strings.Contains(err.Error(), "uninstall") {
					t.Fatalf("legacy format accepted: %v", err)
				}
			}
		})
	}
}

func TestSingleFileCannotDeclareNativeHooks(t *testing.T) {
	source := "hook.style.md"
	pkg, err := LoadSnapshot("fixture", map[string][]byte{source: []byte("---\n" + nativeHookHeader + "target: system\nnative_hooks: [{id: hook, codex: {event: Stop, command: notify}}]\n---\nBody.\n")}, source)
	if err != nil {
		t.Fatal(err)
	}
	if ds := validateWithNative(pkg, testNativeCatalog); !hasDiagnostic(ds, "invalid-schema", SeverityError) {
		t.Fatal(ds)
	}
}
