package codex

import (
	"reflect"
	"testing"
)

func TestExplicitSkillsPreservesOrderAndDeduplicates(t *testing.T) {
	got := ExplicitSkills("Use $review, then $lint; revisit $review and $review-fast.")
	want := []string{"review", "lint", "review-fast"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExplicitSkills() = %#v, want %#v", got, want)
	}
}

func TestExplicitSkillsUsesWordBoundaryFromStyleSpec(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   []string
	}{
		{name: "start", prompt: "$review", want: []string{"review"}},
		{name: "punctuation", prompt: "please ($review) and $lint.", want: []string{"review", "lint"}},
		{name: "hyphen and underscore", prompt: "$review-v2 $lint_rules", want: []string{"review-v2", "lint_rules"}},
		{name: "invalid first character", prompt: "$-review $_review", want: nil},
		{name: "numeric first character is allowed by the spec", prompt: "$9patch", want: []string{"9patch"}},
		{name: "identifier adjacency", prompt: "prefix$review foo_$lint $$nested", want: nil},
		{name: "empty", prompt: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExplicitSkills(tt.prompt); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExplicitSkills(%q) = %#v, want %#v", tt.prompt, got, tt.want)
			}
		})
	}
}
