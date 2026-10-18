package tokenestimate

import "testing"

func TestEstimate(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"abcd", 1},
		{"你好", 2},
		{"!", 1},
		{"a!", 1},
		{"abcde", 2},
	} {
		if got := Estimate(tc.text); got != tc.want {
			t.Errorf("Estimate(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestCountTextCountsUnicodeCodePoints(t *testing.T) {
	if got, want := CountText("你好!"), (Count{Tokens: 3, Chars: 3}); got != want {
		t.Fatalf("CountText() = %#v, want %#v", got, want)
	}
}

func TestEstimateTemplateUsesStaticValuesAndRuntimePlaceholders(t *testing.T) {
	context := Context{
		Package: map[string]any{
			"settings": []any{
				map[string]any{"name": "language", "default": "日本語"},
				map[string]any{"name": "tone", "default": "precise"},
			},
		},
	}
	got, err := EstimateTemplate("{{settings.language}} {{settings.tone}} {{model}} {{harness.cwd}}", context)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderForEstimate("{{settings.language}} {{settings.tone}} {{model}} {{harness.cwd}}", context)
	if err != nil {
		t.Fatal(err)
	}
	if got != CountText(rendered) {
		t.Fatalf("EstimateTemplate() = %#v, rendered count = %#v", got, CountText(rendered))
	}
	if got.Chars != 3+1+7+1+64+1+128 {
		t.Fatalf("estimated chars = %d; rendered=%q", got.Chars, rendered)
	}
}

func TestEstimateTemplateUsesDefaultForUnavailableStaticName(t *testing.T) {
	got, err := EstimateTemplate("{{settings.missing | default: \"Fallback\"}}", Context{})
	if err != nil {
		t.Fatal(err)
	}
	if want := CountText("Fallback"); got != want {
		t.Fatalf("EstimateTemplate() = %#v, want %#v", got, want)
	}
}

func TestEstimateTemplateRejectsMalformedInput(t *testing.T) {
	if _, err := EstimateTemplate("{{model", Context{}); err == nil {
		t.Fatal("EstimateTemplate() accepted malformed input")
	}
}
