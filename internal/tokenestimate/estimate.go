// Package tokenestimate contains the STYLE offline token estimate shared by
// validation, planning, and compilation.
package tokenestimate

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/inxups/style-cli/core/template"
)

// Count contains the offline estimate and exact Unicode character count.
type Count struct {
	Tokens int
	Chars  int
}

// Context contains install-time values used by STYLE templates.
type Context struct {
	Package   map[string]any
	Rule      map[string]any
	Overrides map[string]any
}

// Estimate returns ceil(A/4 + C + 0.5*P), where A is ASCII letters/digits,
// C is CJK Unified Ideographs, kana, or hangul, and P is every other rune.
func Estimate(text string) int {
	ascii, cjk, other := 0, 0, 0
	for _, r := range text {
		switch {
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			ascii++
		case unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana):
			cjk++
		default:
			other++
		}
	}
	// ceil(ascii/4 + cjk + other/2), keeping the calculation integral.
	return (ascii*2 + cjk*8 + other*4 + 7) / 8
}

// CountText estimates model tokens and counts Unicode code points.
func CountText(text string) Count {
	return Count{Tokens: Estimate(text), Chars: utf8.RuneCountInString(text)}
}

// EstimateTemplate renders static values and conservative runtime placeholders
// before estimating the resulting model-facing text.
func EstimateTemplate(source string, context Context) (Count, error) {
	rendered, err := RenderForEstimate(source, context)
	if err != nil {
		return Count{}, err
	}
	return CountText(rendered), nil
}

// RenderForEstimate substitutes conservative runtime placeholders and
// install-time values, allowing callers to apply transforms such as warn's
// first-sentence extraction before counting.
func RenderForEstimate(source string, context Context) (string, error) {
	parsed, err := template.Parse(source)
	if err != nil {
		return "", err
	}
	var rendered strings.Builder
	for _, part := range parsed.Parts() {
		if part.Kind == template.TextPart {
			rendered.WriteString(part.Text)
			continue
		}
		if template.IsEventVariable(part.Name) {
			rendered.WriteString(template.EstimateEvent(part.Name))
			continue
		}
		if part.Name == "provider" {
			// Identifiers can contain punctuation; count the worst allowed width and
			// the explicit fallback conservatively, without changing legacy estimates.
			rendered.WriteString(strings.Repeat(".", 64))
			if part.HasDefault {
				rendered.WriteString(part.Default)
			}
			continue
		}
		if template.IsRuntimeVariable(part.Name) {
			length := 64
			if part.Name == "harness.cwd" {
				length = 128
			}
			rendered.WriteString(strings.Repeat("x", length))
			continue
		}
		if value, ok := StaticValue(context, part.Name); ok {
			rendered.WriteString(value)
		} else if part.HasDefault {
			rendered.WriteString(part.Default)
		}
	}
	return rendered.String(), nil
}

// StaticValue resolves the package and rule variables available at install
// time. The caller supplies source-level package and rule maps.
func StaticValue(context Context, name string) (string, bool) {
	switch {
	case strings.HasPrefix(name, "vars."):
		vars, _ := context.Package["vars"].(map[string]any)
		value, ok := vars[strings.TrimPrefix(name, "vars.")]
		if ok {
			return fmt.Sprint(value), true
		}
	case strings.HasPrefix(name, "settings."):
		settings, _ := context.Package["settings"].([]any)
		settingName := strings.TrimPrefix(name, "settings.")
		for _, setting := range settings {
			fields, _ := setting.(map[string]any)
			if fields != nil && fields["name"] == settingName {
				if value, exists := fields["default"]; exists {
					return fmt.Sprint(value), true
				}
				return "", true
			}
		}
	case name == "package.name" || name == "package.version":
		value, ok := context.Package[strings.TrimPrefix(name, "package.")]
		if ok {
			return fmt.Sprint(value), true
		}
	case name == "rule.id":
		value, ok := context.Rule["id"]
		if !ok {
			value, ok = context.Overrides["id"]
		}
		if ok {
			return fmt.Sprint(value), true
		}
	}
	return "", false
}
