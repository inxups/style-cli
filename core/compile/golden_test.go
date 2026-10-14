package compile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

const fixtureLock = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func buildExampleIndex(t *testing.T) (Result, []byte) {
	t.Helper()
	root := filepath.Join("..", "..", "example", "terse-reviewer")
	pkg, err := spec.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := spec.Validate(pkg); hasErrors(diagnostics) {
		t.Fatalf("example validation failed: %#v", diagnostics)
	}
	resolved, err := spec.Merge(pkg)
	if err != nil {
		t.Fatal(err)
	}
	report, err := plan.Build(resolved, compileAdapter{name: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Build(Request{StyleVersion: "0.3.0", Packages: []PackageInput{{
		Package: resolved, Sequence: 1, Lock: fixtureLock,
		Settings: map[string]string{"max_findings": "8"}, Plans: []plan.Report{report},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Encode(result.Index)
	if err != nil {
		t.Fatal(err)
	}
	return result, raw
}

func TestExampleIndexMatchesByteGoldenAndSchema(t *testing.T) {
	result, raw := buildExampleIndex(t)
	schema := loadIndexSchema(t)
	if err := schema.Validate(decodeJSON(t, raw)); err != nil {
		t.Fatalf("compiled index does not match schema: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "testdata", "golden", "index.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, want) {
		t.Fatalf("compiled index differs from %s; set UPDATE_GOLDEN=1 only for an intentional format change", goldenPath)
	}
	if len(result.StaticBodies) != 1 || result.StaticBodies[0].Harness != "codex" || result.StaticBodies[0].Slot != "developer_instructions" {
		t.Fatalf("static installation output = %#v", result.StaticBodies)
	}
}

func TestIndexSchemaRejectsInvalidDocuments(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema := loadIndexSchema(t)
	cases := []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing required header", func(doc map[string]any) { delete(doc, "format") }},
		{"unknown evaluation point", func(doc map[string]any) { events(doc)["future.event"] = []any{} }},
		{"session entry without sources", func(doc map[string]any) { delete(entry(doc, plan.SessionStart, 0), "sources") }},
		{"session entry with match", func(doc map[string]any) {
			entry(doc, plan.SessionStart, 0)["match"] = map[string]any{"canonical": []any{"shell"}}
		}},
		{"prompt entry with sources", func(doc map[string]any) { entry(doc, plan.PromptSubmitted, 0)["sources"] = []any{"startup"} }},
		{"tool entry without match", func(doc map[string]any) { delete(entry(doc, plan.ToolPre, 0), "match") }},
		{"unknown entry property", func(doc map[string]any) { entry(doc, plan.SessionStart, 0)["body_text"] = "unknown" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document := decodeJSON(t, data).(map[string]any)
			tc.edit(document)
			if err := schema.Validate(document); err == nil {
				t.Fatal("schema accepted an invalid index")
			}
		})
	}
}

func TestIndexSchemaAllowsNegativePriorityAndDuplicateSources(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	document := decodeJSON(t, data).(map[string]any)
	entry(document, plan.SessionStart, 0)["order"].([]any)[0] = -1.25
	entry(document, plan.SessionStart, 0)["sources"] = []any{"startup", "startup"}
	if err := loadIndexSchema(t).Validate(document); err != nil {
		t.Fatalf("schema rejected values accepted by STYLE: %v", err)
	}
}

func loadIndexSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", "index.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode index schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve index schema: %v", err)
	}
	return resolved
}

func decodeJSON(t *testing.T, data []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func events(document map[string]any) map[string]any {
	return document["harnesses"].(map[string]any)["codex"].(map[string]any)
}

func entry(document map[string]any, event string, index int) map[string]any {
	return events(document)[event].([]any)[index].(map[string]any)
}
