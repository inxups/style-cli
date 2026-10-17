package compile

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/inxups/style-cli/schema"
)

func TestM3A13EffortMappingFormatSchemaAndSnapshot(t *testing.T) {
	mapping := EffortMapping{"codex": {"ultra": "xhigh", "future": "high"}}
	pkg := loadResolved(t, "mapped", "  - {id: r, target: event, event: tool.pre, reads: {model: true}, when: {model: unknown}, text: unknown}\n")
	result, err := Build(Request{StyleVersion: "x", Packages: []PackageInput{inputFor(t, pkg, 1)}, EffortMapping: mapping})
	if err != nil || result.Index.Format != 6 {
		t.Fatal(result.Index.Format, err)
	}
	digest := result.Index.EffortMapping.SHA256("codex")
	mapping["codex"]["ultra"] = "low"
	if result.Index.EffortMapping["codex"]["ultra"] != "xhigh" || digest == mapping.SHA256("codex") {
		t.Fatal("mapping was borrowed or digest ignored contents")
	}
	data, _ := Encode(result.Index)
	if schema.ValidateIndex(data) != nil || json.Unmarshal(data, new(Index)) != nil {
		t.Fatal("format6 rejected")
	}
	for _, format := range []string{"1", "2", "3", "4", "5"} {
		old := bytes.Replace(data, []byte(`"format": 6`), []byte(`"format": `+format), 1)
		if schema.ValidateIndex(old) == nil || json.Unmarshal(old, new(Index)) == nil {
			t.Fatal("old format accepted mapping", format)
		}
	}
	for _, invalid := range []EffortMapping{{"codex": {"Future": "high"}}, {"codex": {"": "high"}}, {"codex": {"future": "unknown"}}, {"codex": {}}, {"../private": {"x": "high"}}} {
		if invalid.Validate() == nil {
			t.Fatal("invalid normalized mapping", invalid)
		}
	}
}
