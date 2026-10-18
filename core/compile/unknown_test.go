package compile

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/inxups/style-cli/schema"
)

func TestM3A12UnknownCompileWholeFormat5Gate(t *testing.T) {
	for _, field := range []string{"model: unknown", "provider: unknown", "thinking_effort: unknown", "harness: {permission_mode: unknown}"} {
		t.Run(field, func(t *testing.T) {
			rules := "  - {id: missing, target: event, event: tool.pre, when: {" + field + "}, text: missing}\n  - {id: provider, target: event, event: tool.pre, text: '{{provider | default: \"missing\"}}'}\n  - {id: fenced, target: event, event: prompt.submitted, refresh: 2, reason: fixture, text: '{{event.prompt}}'}\n"
			pkg := loadResolved(t, "unknown", rules)
			result, err := Build(Request{StyleVersion: "0.3.0", Packages: []PackageInput{inputFor(t, pkg, 1)}})
			if err != nil || result.Index.Format != UnknownFormat {
				t.Fatal(result.Index.Format, err)
			}
			data, err := Encode(result.Index)
			if err != nil || schema.ValidateIndex(data) != nil || json.Unmarshal(data, new(Index)) != nil {
				t.Fatal(err, string(data))
			}
			for _, format := range []string{"1", "2", "3", "4"} {
				old := bytes.Replace(data, []byte(`"format": 5`), []byte(`"format": `+format), 1)
				if schema.ValidateIndex(old) == nil || json.Unmarshal(old, new(Index)) == nil {
					t.Fatal("accepted unknown in old format", format)
				}
			}
			for _, spelling := range []string{"UnKnOwN", " unknown "} {
				if field != "model: unknown" && field != "thinking_effort: unknown" || field == "model: unknown" && spelling == " unknown " {
					continue
				}
				raw := bytes.ReplaceAll(data, []byte(`"unknown"`), []byte(`"`+spelling+`"`))
				raw = bytes.Replace(raw, []byte(`"format": 5`), []byte(`"format": 4`), 1)
				if schema.ValidateIndex(raw) == nil || json.Unmarshal(raw, new(Index)) == nil {
					t.Fatal("raw selector evaded format gate", spelling)
				}
			}
		})
	}
	// Raw mixed-case model and whitespace effort spellings must not evade gates.
	data := []byte(`{"format":4,"style_version":"x","packages":[],"static":{},"harnesses":{}}`)
	if schema.ValidateIndex(data) != nil {
		t.Fatal("legal format 4 compatibility")
	}
}
