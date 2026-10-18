package template

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"
)

// EventMaxRunes bounds each explicitly referenced untrusted event value.
const EventMaxRunes = 512

// IsEventVariable accepts only the explicit, fenced event namespace.
func IsEventVariable(name string) bool {
	if !strings.HasPrefix(name, "event.") {
		return false
	}
	return EventFieldKnown(strings.TrimPrefix(name, "event."))
}

// EventFieldKnown is shared by condition and template validation.
func EventFieldKnown(field string) bool {
	switch field {
	case "prompt", "tool.name", "tool.input", "tool.added", "tool.output", "mcp.server", "mcp.tool", "skill.name":
		return true
	}
	if !strings.HasPrefix(field, "tool.input.") {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(field, "tool.input."), ".") {
		if segment == "" {
			return false
		}
		for _, r := range segment {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
				return false
			}
		}
	}
	return true
}

// FenceEvent renders a single-line JSON string inside a fixed data boundary.
// It never interprets event content as template syntax or trusted instructions.
func FenceEvent(name, value string) string {
	var prefix strings.Builder
	count, truncated := 0, false
	for _, r := range value {
		if count == EventMaxRunes {
			truncated = true
			break
		}
		prefix.WriteRune(r)
		count++
	}
	raw, _ := json.Marshal(prefix.String())
	var encoded strings.Builder
	for _, r := range string(raw) {
		if r == '`' || unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			if r <= 0xffff {
				fmt.Fprintf(&encoded, `\u%04x`, r)
			} else {
				hi, lo := utf16.EncodeRune(r)
				fmt.Fprintf(&encoded, `\u%04x\u%04x`, hi, lo)
			}
		} else {
			encoded.WriteRune(r)
		}
	}
	return eventEnvelope(name, encoded.String(), truncated)
}

func eventEnvelope(name, encoded string, truncated bool) string {
	return fmt.Sprintf("<style-untrusted-event field=%q truncated=%q>\n%s\n</style-untrusted-event>", strings.TrimPrefix(name, "event."), fmt.Sprint(truncated), encoded)
}

// EstimateEvent uses the maximum JSON escape width per Unicode code point.
// The false flag is longer than true; actual runtime caps use complete output.
func EstimateEvent(name string) string {
	return eventEnvelope(name, `"`+strings.Repeat(`\udb40\udc01`, EventMaxRunes)+`"`, false)
}
