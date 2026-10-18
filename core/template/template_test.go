package template

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseClassifiesPartsAndEscapedBraces(t *testing.T) {
	parsed, err := Parse(`Hello \{{literal}} {{settings.lang}} / {{model | default: "unknown"}}.`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Kind: TextPart, Text: "Hello {{literal}} "},
		{Kind: VariablePart, Name: "settings.lang"},
		{Kind: TextPart, Text: " / "},
		{Kind: VariablePart, Name: "model", Default: "unknown", HasDefault: true},
		{Kind: TextPart, Text: "."},
	}
	if got := parsed.Parts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("parts = %#v, want %#v", got, want)
	}
	if !parsed.IsDynamic() || parsed.Classify() != Dynamic {
		t.Fatal("model variable should classify the template as dynamic")
	}
	static, err := Parse(`{{settings.lang}}/{{settings.detail}}`)
	if err != nil {
		t.Fatal(err)
	}
	if static.IsDynamic() || static.Classify() != Static {
		t.Fatal("install-time variables should classify the template as static")
	}
}

func TestRenderDefaultsAndStaticGuard(t *testing.T) {
	parsed, err := Parse(`{{model}}|{{model | default: "unknown"}}|{{settings.lang}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Render(map[string]string{"model": "gpt-5"}); got != "gpt-5|gpt-5|" {
		t.Fatalf("Render missing value = %q", got)
	}
	if got := parsed.Render(map[string]string{"model": "", "settings.lang": "zh"}); got != "||zh" {
		t.Fatalf("Render present empty value = %q", got)
	}
	if _, err := parsed.RenderStatic(map[string]string{}); err == nil {
		t.Fatal("RenderStatic should reject dynamic templates")
	}
	static, err := Parse(`{{settings.lang}}/{{settings.detail | default: 'concise'}}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := static.RenderStatic(map[string]string{"settings.lang": "zh"})
	if err != nil || got != "zh/concise" {
		t.Fatalf("RenderStatic = %q, %v", got, err)
	}
}

func TestParseRejectsInvalidExpressions(t *testing.T) {
	for _, source := range []string{
		"{{missing",
		"{{1bad}}",
		"{{model | default: unknown}}",
		"{{model | default: \"x\" | extra: \"y\"}}",
		"{{model | other: \"x\"}}",
	} {
		if _, err := Parse(source); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", source)
		}
	}
	if _, err := Parse(`{{model | default: "unterminated}}`); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("unterminated quoted default error = %v", err)
	}
}

func TestRuntimeVariableClassification(t *testing.T) {
	for _, name := range []string{"model", "thinking_effort", "thinking_effort_raw", "harness.name", "tool.input.command", "prompt"} {
		if !IsRuntimeVariable(name) {
			t.Errorf("IsRuntimeVariable(%q) = false", name)
		}
	}
	if IsRuntimeVariable("settings.lang") {
		t.Fatal("settings.lang is an install-time variable")
	}
}
