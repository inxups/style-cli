package spec

import (
	"reflect"
	"testing"
)

func TestValidateRejectsNonStringNestedYAMLKeys(t *testing.T) {
	for _, field := range []string{"budget", "defaults", "when"} {
		t.Run(field, func(t *testing.T) {
			metadata := ""
			rules := "  - id: base\n    target: system\n    text: body\n"
			switch field {
			case "budget":
				metadata = "budget: {1: 10}\n"
			case "defaults":
				metadata = "defaults: {1: inject}\n"
			case "when":
				rules = "  - id: base\n    target: system\n    text: body\n    when: {conditions: [{1: value}]}\n"
			}
			diagnostics := loadAndValidate(t, testManifest("map-key-case", metadata, rules), nil)
			if !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
				t.Fatalf("Validate() diagnostics = %#v, want invalid-schema", diagnostics)
			}
		})
	}
}

func TestValidateRejectsNonStringKeysInRuleFilesAndOverrides(t *testing.T) {
	for _, override := range []bool{false, true} {
		rules := "  - from: rules/base.md\n"
		file := "---\ntarget: native\ncodex: {nested: {1: value}}\n---\nbody\n"
		if override {
			rules += "    when: {conditions: [{1: value}]}\n"
			file = "---\ntarget: system\n---\nbody\n"
		}
		diagnostics := loadAndValidate(t, testManifest("rule-key-case", "", rules), map[string]string{"rules/base.md": file})
		if !hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
			t.Fatalf("override=%v diagnostics = %#v, want invalid-schema", override, diagnostics)
		}
	}
}

func TestAsStringMapRejectsLossyConversion(t *testing.T) {
	if got := asStringMap(map[any]any{"valid": true, 1: "invalid"}); got != nil {
		t.Fatalf("invalid mapping converted to %#v", got)
	}
	if got := asStringMap(map[any]any{"1": "value"}); !reflect.DeepEqual(got, map[string]any{"1": "value"}) {
		t.Fatalf("valid string keys changed: %#v", got)
	}
	if diagnostics := loadAndValidate(t, testManifest("string-key", "budget: {'per_rule': 100}\n", "  - id: base\n    target: system\n    text: body\n"), nil); hasDiagnostic(diagnostics, "invalid-schema", SeverityError) {
		t.Fatalf("quoted string key rejected: %#v", diagnostics)
	}
}
