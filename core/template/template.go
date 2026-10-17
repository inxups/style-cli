package template

import (
	"fmt"
	"regexp"
	"strings"
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

// PartKind identifies a literal text or variable part of a parsed template.
type PartKind uint8

const (
	// TextPart contains literal output text.
	TextPart PartKind = iota
	// VariablePart contains a variable reference and its optional default.
	VariablePart
)

// Part is one part of a parsed template. For TextPart, Text contains the
// literal output. For VariablePart, Name contains the variable name, and
// Default/HasDefault describe its optional fallback value.
type Part struct {
	Kind       PartKind
	Text       string
	Name       string
	Default    string
	HasDefault bool
}

// Expression is a single variable expression without the surrounding braces.
// ParseExpression is useful to validators that need the name and whether a
// fallback was supplied without parsing a complete body.
type Expression struct {
	Name       string
	Default    string
	HasDefault bool
}

// ParseExpression parses the contents of {{...}}. Whitespace around the
// variable and modifier is ignored.
func ParseExpression(expression string) (Expression, error) {
	part, err := parseVariable(expression, 0)
	if err != nil {
		return Expression{}, err
	}
	return Expression{Name: part.Name, Default: part.Default, HasDefault: part.HasDefault}, nil
}

// Template is a parsed template. Its parts are immutable to callers.
type Template struct {
	parts []Part
}

// Parse parses replacement-only template syntax. It accepts {{name}} and
// {{name | default: "value"}} (single-quoted defaults are also accepted).
// The sequence \{{ emits a literal {{ without the escape backslash.
func Parse(source string) (*Template, error) {
	parts := make([]Part, 0)
	var literal strings.Builder
	flushLiteral := func() {
		if literal.Len() == 0 {
			return
		}
		parts = append(parts, Part{Kind: TextPart, Text: literal.String()})
		literal.Reset()
	}

	for i := 0; i < len(source); {
		if source[i] == '\\' && strings.HasPrefix(source[i+1:], "{{") {
			literal.WriteString("{{")
			i += 3
			continue
		}
		if !strings.HasPrefix(source[i:], "{{") {
			literal.WriteByte(source[i])
			i++
			continue
		}

		flushLiteral()
		end, err := findExpressionEnd(source, i+2)
		if err != nil {
			return nil, err
		}
		part, err := parseVariable(source[i+2:end], i)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
		i = end + 2
	}
	flushLiteral()
	return &Template{parts: parts}, nil
}

// Parts returns a copy of the parsed parts in source order.
func (t *Template) Parts() []Part {
	if t == nil {
		return nil
	}
	return append([]Part(nil), t.parts...)
}

// Classification describes whether a template can be rendered at install
// time or needs runtime values.
type Classification uint8

const (
	// Static means that all variable references are install-time values.
	Static Classification = iota
	// Dynamic means that at least one variable reference is runtime data.
	Dynamic
)

// Classify returns Dynamic if any variable part references a runtime variable;
// otherwise it returns Static.
func (t *Template) Classify() Classification {
	if t == nil {
		return Static
	}
	for _, part := range t.parts {
		if part.Kind == VariablePart && IsRuntimeVariable(part.Name) {
			return Dynamic
		}
	}
	return Static
}

// IsDynamic reports whether the template references at least one runtime
// variable.
func (t *Template) IsDynamic() bool { return t.Classify() == Dynamic }

// IsStatic reports whether the template contains only install-time variables
// and literal text.
func (t *Template) IsStatic() bool { return t.Classify() == Static }

// IsRuntimeVariable reports whether name denotes runtime data in STYLE M1.
// It includes variables that validation may separately reject from templates
// (such as event-content variables), so classification never treats runtime
// data as install-time data.
func IsRuntimeVariable(name string) bool {
	switch name {
	case "model", "thinking_effort", "thinking_effort_raw",
		"prompt", "tool.name", "tool.input", "tool.added", "tool.output",
		"mcp.server", "mcp.tool", "skill.name":
		return true
	}
	return strings.HasPrefix(name, "harness.") || strings.HasPrefix(name, "tool.input.") || IsEventVariable(name)
}

// Render renders the template with values from context. A missing value uses
// the variable's default, or the empty string when no default is declared. An
// explicitly present empty string is preserved as an available value.
func (t *Template) Render(context map[string]string) string {
	if t == nil {
		return ""
	}
	var rendered strings.Builder
	for _, part := range t.parts {
		if part.Kind == TextPart {
			rendered.WriteString(part.Text)
			continue
		}
		value, ok := context[part.Name]
		if !ok {
			if part.HasDefault {
				value = part.Default
			} else {
				value = ""
			}
		}
		if IsEventVariable(part.Name) {
			value = FenceEvent(part.Name, value)
		}
		rendered.WriteString(value)
	}
	return rendered.String()
}

// RenderStatic renders the template only when it is classified as static.
// It returns an error for dynamic templates to prevent accidental compile-time
// substitution of runtime data.
func (t *Template) RenderStatic(context map[string]string) (string, error) {
	if t.IsDynamic() {
		return "", fmt.Errorf("cannot render dynamic template as static")
	}
	return t.Render(context), nil
}

func findExpressionEnd(source string, start int) (int, error) {
	var quote byte
	for i := start; i < len(source); i++ {
		if quote != 0 {
			if source[i] == quote {
				quote = 0
			}
			continue
		}
		if source[i] == '\'' || source[i] == '"' {
			quote = source[i]
			continue
		}
		if i+1 < len(source) && source[i] == '}' && source[i+1] == '}' {
			return i, nil
		}
	}
	if quote != 0 {
		return 0, fmt.Errorf("template expression starting at byte %d has an unterminated quoted default", start-2)
	}
	return 0, fmt.Errorf("template expression starting at byte %d has no closing }}", start-2)
}

func parseVariable(expression string, offset int) (Part, error) {
	separator := findSeparator(expression)
	name := strings.TrimSpace(expression)
	if separator >= 0 {
		name = strings.TrimSpace(expression[:separator])
	}
	if !variableNamePattern.MatchString(name) {
		return Part{}, fmt.Errorf("invalid template variable %q at byte %d", name, offset)
	}
	part := Part{Kind: VariablePart, Name: name}
	if separator < 0 {
		return part, nil
	}

	defaultExpr := strings.TrimSpace(expression[separator+1:])
	if !strings.HasPrefix(defaultExpr, "default:") {
		return Part{}, fmt.Errorf("invalid template modifier %q at byte %d", defaultExpr, offset)
	}
	valueExpr := strings.TrimSpace(strings.TrimPrefix(defaultExpr, "default:"))
	if len(valueExpr) < 2 || (valueExpr[0] != '\'' && valueExpr[0] != '"') || valueExpr[len(valueExpr)-1] != valueExpr[0] {
		return Part{}, fmt.Errorf("template default must be a quoted string at byte %d", offset)
	}
	for i := 1; i < len(valueExpr)-1; i++ {
		if valueExpr[i] == valueExpr[0] {
			return Part{}, fmt.Errorf("template default must be a single quoted string at byte %d", offset)
		}
	}
	part.Default = valueExpr[1 : len(valueExpr)-1]
	part.HasDefault = true
	return part, nil
}

func findSeparator(expression string) int {
	var quote byte
	for i := 0; i < len(expression); i++ {
		if quote != 0 {
			if expression[i] == quote {
				quote = 0
			}
			continue
		}
		if expression[i] == '\'' || expression[i] == '"' {
			quote = expression[i]
			continue
		}
		if expression[i] == '|' {
			return i
		}
	}
	return -1
}
