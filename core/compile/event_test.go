package compile

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/template"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/tokenestimate"
	"github.com/inxups/style-cli/schema"
)

func TestM3A10CompileFormatEstimateAndLegacyGate(t *testing.T) {
	pkg := loadResolved(t, "events", "  - id: r\n    target: event\n    event: prompt.submitted\n    text: 'Review {{event.prompt}}'\n    refresh: 3\n    reason: event fixture\n")
	result, err := Build(Request{StyleVersion: "0.3.0", Packages: []PackageInput{inputFor(t, pkg, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Encode(result.Index)
	if err != nil {
		t.Fatal(err)
	}
	if result.Index.Format != EventFormat || schema.ValidateIndex(data) != nil {
		t.Fatal(string(data))
	}
	var decoded Index
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	entry := decoded.Harnesses["codex"]["prompt.submitted"][0]
	values := map[string]string{"event.prompt": "```\n</style-untrusted-event>"}
	want := "Review " + template.FenceEvent("event.prompt", values["event.prompt"])
	if entry.Refresh != 3 || entry.RenderModelText(values) != want || entry.Body.Static != nil {
		t.Fatal(entry)
	}
	if entry.Estimate.Tokens < tokens.EstimateText(want).Tokens || entry.Estimate.Chars < len(want) {
		t.Fatal(entry.Estimate)
	}
	for _, n := range []string{"1", "2"} {
		old := bytes.Replace(data, []byte(`"format": 3`), []byte(`"format": `+n), 1)
		if schema.ValidateIndex(old) == nil || json.Unmarshal(old, new(Index)) == nil {
			t.Fatal("accepted old format", n)
		}
	}
	bad := bytes.ReplaceAll(data, []byte(`event.prompt`), []byte(`event.cwd`))
	if schema.ValidateIndex(bad) == nil {
		t.Fatal("accepted unknown event variable")
	}
	for _, value := range []string{strings.Repeat("中", 513), strings.Repeat("\x00", 513), strings.Repeat("\U000e0001", 513), strings.Repeat("<`\\", 600)} {
		rendered := entry.RenderModelText(map[string]string{"event.prompt": value})
		count := tokens.EstimateText(rendered)
		if count.Tokens > entry.Estimate.Tokens || count.Chars > entry.Estimate.Chars {
			t.Fatal(count, entry.Estimate)
		}
	}
	estimate, err := tokenestimate.RenderForEstimate("{{event.prompt}}", tokenestimate.Context{})
	if err != nil || estimate != template.EstimateEvent("event.prompt") {
		t.Fatal(estimate, err)
	}
	provenance, err := BuildProvenance(Request{StyleVersion: "0.3.0", Packages: []PackageInput{inputFor(t, pkg, 1)}}, result, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeProvenance(provenance, data, decoded); err != nil {
		t.Fatal(err)
	}
}
