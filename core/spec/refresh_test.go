package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshValidation(t *testing.T) {
	for _, value := range []string{"null", "true", "false", "'3'", "-1", "1.5", "3.0", "1000001", "[]", "{}"} {
		t.Run(value, func(t *testing.T) {
			for _, defaults := range []bool{false, true} {
				fields, rules := "", "  - id: base\n    target: system\n    text: body\n"
				if defaults {
					fields = "defaults: {refresh: " + value + "}\n"
				} else {
					rules += "    refresh: " + value + "\n"
				}
				if ds := loadAndValidate(t, testManifest("demo", fields, rules), nil); !hasDiagnostic(ds, "refresh-invalid", SeverityError) {
					t.Fatalf("%v", ds)
				}
			}
		})
	}
	for _, tc := range []struct{ fields, want string }{
		{"refresh: 0", ""}, {"refresh: 1\n    reason: periodic", ""}, {"refresh: 1000000\n    reason: periodic", ""},
		{"refresh: 1", "T007"}, {"refresh: 1\n    reason: ' '", "T007"},
		{"refresh: 1\n    repeat: every", "refresh-incompatible"}, {"refresh: 1\n    repeat: turn", "refresh-incompatible"},
		{"refresh: 1\n    action: warn", "refresh-incompatible"}, {"refresh: 1\n    action: warn\n    repeat: session", ""},
		{"refresh: 1\n    mode: replace", "refresh-incompatible"}, {"refresh: 1\n    action: block", "refresh-incompatible"},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			ds := loadAndValidate(t, testManifest("demo", "", "  - id: base\n    target: system\n    text: body\n    "+tc.fields+"\n"), nil)
			if tc.want == "" {
				for _, d := range ds {
					if d.Severity == SeverityError {
						t.Fatal(ds)
					}
				}
			} else if !hasDiagnostic(ds, tc.want, map[bool]string{true: SeverityWarning, false: SeverityError}[tc.want == "T007"]) {
				t.Fatal(ds)
			}
		})
	}
	for _, n := range []int{0, 1} {
		rules := fmt.Sprintf("native_hooks:\n  - id: native\n    refresh: %d\n    codex: {event: SessionStart, command: 'echo ok'}\n", n)
		if ds := loadAndValidate(t, nativeHookHeader+rules, nil); !hasDiagnostic(ds, "native-field", SeverityError) {
			t.Fatal(ds)
		}
	}
	rules := "  - id: child\n    target: event\n    event: subagent.start\n    text: body\n    refresh: 1\n"
	if ds := loadAndValidate(t, testManifest("demo", "", rules), nil); !hasDiagnostic(ds, "refresh-incompatible", SeverityError) {
		t.Fatal(ds)
	}
}

func TestRefreshDefaultsFromAndSingleFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "rules/base.md", "---\ntarget: system\nrefresh: 5\nreason: periodic\n---\nbody\n")
	for _, tc := range []struct {
		override string
		want     int
	}{{"", 5}, {"    refresh: 0\n", 0}, {"    refresh: 7\n", 7}} {
		writeFile(t, root, "STYLE.yaml", testManifest("demo", "defaults: {refresh: 3}\n", "  - from: rules/base.md\n"+tc.override))
		pkg, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range Validate(pkg) {
			if d.Severity == SeverityError {
				t.Fatal(d)
			}
		}
		merged, err := Merge(pkg)
		if err != nil {
			t.Fatal(err)
		}
		if n, ok := RefreshInterval(merged.Rules[0].Effective["refresh"]); !ok || n != tc.want {
			t.Fatalf("%d %v want %d", n, ok, tc.want)
		}
	}
	file := filepath.Join(t.TempDir(), "one.style.md")
	if err := os.WriteFile(file, []byte("---\nspec: style/v0.3\nname: single\nversion: 1.0.0\ndescription: single\ntarget: system\nrefresh: 3\nreason: periodic\n---\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pkg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range Validate(pkg) {
		if d.Severity == SeverityError {
			t.Fatal(d)
		}
	}
	for _, value := range []any{uint64(^uint64(0)), int64(-1), float64(1), nil, true, "1"} {
		if _, ok := RefreshInterval(value); ok {
			t.Fatalf("accepted %#v", value)
		}
	}
}
