package spec

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestValidateRejectsNonFiniteNumbers(t *testing.T) {
	for _, number := range []string{".nan", ".inf", "-.inf"} {
		cases := []struct{ name, metadata, ruleFields, override string }{
			{name: "package priority", metadata: "priority: " + number + "\n"},
			{name: "defaults priority", metadata: "defaults: {priority: " + number + "}\n"},
			{name: "defaults max tokens", metadata: "defaults: {max_tokens: " + number + "}\n"},
			{name: "rule priority", ruleFields: "    priority: " + number + "\n"},
			{name: "rule max tokens", ruleFields: "    max_tokens: " + number + "\n"},
			{name: "override priority", override: "    priority: " + number + "\n"},
			{name: "override max tokens", override: "    max_tokens: " + number + "\n"},
			{name: "vars", metadata: "vars: {number: " + number + "}\n"},
		}
		for _, field := range []string{"per_rule", "per_event", "always_on"} {
			cases = append(cases, struct{ name, metadata, ruleFields, override string }{
				name: "budget " + field, metadata: fmt.Sprintf("budget: {%s: %s}\n", field, number),
			})
		}
		for _, tc := range cases {
			t.Run(number+"/"+tc.name, func(t *testing.T) {
				rules := "  - id: base\n    target: system\n    text: body\n" + tc.ruleFields
				var files map[string]string
				if tc.override != "" {
					rules = "  - from: rules/base.md\n" + tc.override
					files = map[string]string{"rules/base.md": "---\ntarget: system\n---\nbody\n"}
				}
				if diagnostics := loadAndValidate(t, testManifest("finite-case", tc.metadata, rules), files); !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
					t.Fatalf("Validate() diagnostics = %#v, want invalid-schema", diagnostics)
				}
			})
		}
	}
}

func TestValidatedFiniteNumbersEncodeAsJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("finite-values", "priority: 3.5\nvars: {number: 1.5}\nbudget: {per_rule: 100.5}\ndefaults: {priority: 2.5, max_tokens: 50}\n", "  - id: base\n    target: system\n    text: body\n"))
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range Validate(pkg) {
		if diagnostic.Severity == SeverityError {
			t.Fatal(diagnostic)
		}
	}
	resolved, err := Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(resolved.Rules[0].Effective); err != nil {
		t.Fatalf("validated effective fields failed JSON encoding: %v", err)
	}
}
