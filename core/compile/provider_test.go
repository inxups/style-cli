package compile

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/inxups/style-cli/schema"
)

func TestM3A11ProviderCompileFormatSchemaAndOldGate(t *testing.T) {
	for _, rule := range []string{
		"  - {id: r, target: event, event: tool.pre, reads: {provider: true}, when: {provider: custom.ID}, text: service}\n",
		"  - {id: r, target: system, reads: {provider: true}, text: '{{provider | default: \"missing\"}}'}\n",
		"  - {id: r, target: event, event: prompt.submitted, refresh: 3, reason: fixture, reads: {provider: true, event: [prompt]}, text: '{{provider}} {{event.prompt}}'}\n",
	} {
		pkg := loadResolved(t, "provider", rule)
		r, err := Build(Request{StyleVersion: "0.3.0", Packages: []PackageInput{inputFor(t, pkg, 1)}})
		if err != nil {
			t.Fatal(err)
		}
		data, err := Encode(r.Index)
		if err != nil || r.Index.Format != ProviderFormat || schema.ValidateIndex(data) != nil {
			t.Fatal(err, string(data))
		}
		if err := json.Unmarshal(data, new(Index)); err != nil {
			t.Fatal(err)
		}
		for _, version := range []string{"1", "2", "3"} {
			old := bytes.Replace(data, []byte(`"format": 4`), []byte(`"format": `+version), 1)
			if schema.ValidateIndex(old) == nil || json.Unmarshal(old, new(Index)) == nil {
				t.Fatal("legacy provider semantics accepted", version)
			}
		}
		for _, events := range r.Index.Harnesses {
			for _, entries := range events {
				for _, entry := range entries {
					if !entry.HasProvider() {
						t.Fatal(entry)
					}
					if !strings.Contains(entry.RenderModelText(map[string]string{"provider": "custom.ID"}), "service") && !strings.Contains(entry.RenderModelText(map[string]string{"provider": "custom.ID"}), "custom.ID") {
						t.Fatal(entry)
					}
				}
			}
		}
	}
}
