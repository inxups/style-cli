package template

import (
	"reflect"
	"testing"
)

func TestParseExpressionCapturesDefault(t *testing.T) {
	got, err := ParseExpression(` name | default: "a | b" `)
	if err != nil {
		t.Fatal(err)
	}
	want := Expression{Name: "name", Default: "a | b", HasDefault: true}
	if got != want {
		t.Fatalf("ParseExpression() = %#v, want %#v", got, want)
	}
	noDefault, err := ParseExpression(" model ")
	if err != nil || noDefault != (Expression{Name: "model"}) {
		t.Fatalf("ParseExpression without default = %#v, %v", noDefault, err)
	}
}

func TestParseAndRenderEscapesAndDefaults(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		context map[string]string
		want    string
	}{
		{name: "escaped braces", source: `before \{{name}} after`, want: "before {{name}} after"},
		{name: "slash before escaped braces", source: `\\{{name}}`, want: `\{{name}}`},
		{name: "missing value with quoted default", source: `{{model | default: "unknown"}}`, want: "unknown"},
		{name: "empty default", source: `{{model | default: ""}}`, want: ""},
		{name: "present empty value does not use default", source: `{{model | default: "fallback"}}`, context: map[string]string{"model": ""}, want: ""},
		{name: "single quoted default", source: `{{model | default: 'unknown'}}`, want: "unknown"},
		{name: "quoted delimiters", source: `{{model | default: "x }} y | z"}}`, want: "x }} y | z"},
		{name: "unicode literal and value", source: `你好，{{name}}！`, context: map[string]string{"name": "世界"}, want: "你好，世界！"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if got := parsed.Render(tt.context); got != tt.want {
				t.Fatalf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRejectsMalformedExpressions(t *testing.T) {
	for _, source := range []string{
		"{{missing",
		"{{}}",
		"{{9bad}}",
		"{{bad-name}}",
		`{{name | default: nope}}`,
		`{{name | default: "x'}}`,
		`{{name | default: "x" | extra: "y"}}`,
		`{{name | default: "unclosed}}`,
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := Parse(source); err == nil {
				t.Fatalf("Parse(%q) succeeded, want an error", source)
			}
		})
	}
}

func TestPartsAreDetachedAndStaticRenderingUsesDefaults(t *testing.T) {
	parsed, err := Parse(`{{vars.topic}} / {{settings.style | default: "concise"}}`)
	if err != nil {
		t.Fatal(err)
	}
	parts := parsed.Parts()
	want := []Part{
		{Kind: VariablePart, Name: "vars.topic"},
		{Kind: TextPart, Text: " / "},
		{Kind: VariablePart, Name: "settings.style", Default: "concise", HasDefault: true},
	}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("Parts() = %#v, want %#v", parts, want)
	}
	parts[0].Name = "changed"
	if parsed.Parts()[0].Name != "vars.topic" {
		t.Fatal("Parts() returned mutable internal parts")
	}
	got, err := parsed.RenderStatic(map[string]string{"vars.topic": "reviews"})
	if err != nil || got != "reviews / concise" {
		t.Fatalf("RenderStatic() = %q, %v", got, err)
	}
}

func TestRuntimeClassificationIncludesEventValues(t *testing.T) {
	for _, name := range []string{
		"model", "thinking_effort", "thinking_effort_raw", "harness.cwd", "harness.turn_id",
		"prompt", "tool.name", "tool.input", "tool.input.command", "tool.added", "tool.output",
		"mcp.server", "mcp.tool", "skill.name",
	} {
		if !IsRuntimeVariable(name) {
			t.Errorf("IsRuntimeVariable(%q) = false", name)
		}
	}
	for _, name := range []string{"vars.topic", "settings.style", "package.name", "package.version", "rule.id", "unknown"} {
		if IsRuntimeVariable(name) {
			t.Errorf("IsRuntimeVariable(%q) = true", name)
		}
	}
}
