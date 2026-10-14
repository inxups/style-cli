package compile

import (
	"fmt"
	"strings"

	"github.com/inxups/style-cli/core/template"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

func compileBody(source string, context tokenestimate.Context) (Body, error) {
	parsed, err := template.Parse(source)
	if err != nil {
		return Body{}, err
	}
	var literal strings.Builder
	parts := make([]Fragment, 0)
	flush := func() {
		if literal.Len() > 0 {
			parts = append(parts, Fragment{Text: literal.String()})
			literal.Reset()
		}
	}
	for _, part := range parsed.Parts() {
		if part.Kind == template.TextPart {
			literal.WriteString(part.Text)
		} else if template.IsRuntimeVariable(part.Name) {
			flush()
			variable := &Variable{Name: part.Name}
			if part.HasDefault {
				value := part.Default
				variable.Default = &value
			}
			parts = append(parts, Fragment{Variable: variable})
		} else if value, available := tokenestimate.StaticValue(context, part.Name); available {
			literal.WriteString(value)
		} else if part.HasDefault {
			literal.WriteString(part.Default)
		} else {
			return Body{}, fmt.Errorf("compile: static template variable %q has no value", part.Name)
		}
	}
	if !parsed.IsDynamic() {
		value := literal.String()
		return Body{Static: &value}, nil
	}
	flush()
	return Body{Parts: parts}, nil
}

// Render substitutes runtime variables without parsing templates. Missing
// variables use their default; explicitly available empty strings remain empty.
// First-sentence extraction is applied only after substitution.
func (body Body) Render(values map[string]string) string {
	var text strings.Builder
	if body.Static != nil {
		text.WriteString(*body.Static)
	} else {
		for _, part := range body.Parts {
			if part.Variable == nil {
				text.WriteString(part.Text)
				continue
			}
			value, exists := values[part.Variable.Name]
			if !exists && part.Variable.Default != nil {
				value = *part.Variable.Default
			}
			text.WriteString(value)
		}
	}
	return text.String()
}
