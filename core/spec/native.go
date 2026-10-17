package spec

import (
	"fmt"
	"sort"
	"strings"
)

// NativeHarness is the native hook surface one registered harness accepts
// (rules §3.10). Adapters implement it; core/spec never names a harness.
type NativeHarness interface {
	// NativeEvent reports whether event can be written and whether it
	// accepts a matcher.
	NativeEvent(event string) (matcher bool, ok bool)
	// ValidMatcher reports whether matcher is valid in the harness dialect.
	ValidMatcher(matcher string) bool
}

// NativeCatalog maps registered harness names to their native hook surface.
type NativeCatalog map[string]NativeHarness

// NativeRule is one harness mapping of a target: native rule.
type NativeRule struct {
	Package string
	RuleID  string
	Harness string
	Event   string
	Matcher string
	Command string
	Timeout int // seconds; 0 means the adapter default
}

const maxNativeTimeout = 600

// nativeEngineFields are rule fields that only the rule engine interprets.
// Writing one on a native rule is an error; inherited defaults are ignored.
var nativeEngineFields = stringSet("action", "at", "when", "repeat", "reinject_on_compact", "sources", "model_text", "max_tokens", "mode", "reason", "tool", "names", "mcp", "skill", "event")

var nativeMappingFields = stringSet("event", "matcher", "command", "timeout")

func isNativeRule(rule Rule, pkg *Package) bool {
	return effectiveRuleText(rule, pkg, "target", "") == "native"
}

// nativeHarnessKeys returns the keys that are neither STYLE rule fields nor
// package fields, so a native rule treats them as harness mappings. Keys from
// a from: entry replace the rule file's mapping as a whole.
func nativeHarnessKeys(rule Rule) []string {
	seen := make(map[string]bool)
	for _, values := range []map[string]any{rule.Fields, rule.Overrides} {
		for key := range values {
			if !ruleFields[key] && !isPackageField(key) {
				seen[key] = true
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nativeMapping(rule Rule, harness string) (any, bool) {
	if value, ok := rule.Overrides[harness]; ok {
		return value, true
	}
	value, ok := rule.Fields[harness]
	return value, ok
}

// validateNativeRule checks the harness-independent shape of a native rule.
func (v *validator) validateNativeRule(rule Rule, source string) {
	for _, values := range []map[string]any{rule.Fields, rule.Overrides} {
		for _, key := range sortedMapKeys(values) {
			if nativeEngineFields[key] {
				v.add("native-field", source, fmt.Sprintf("native rules cannot set rule engine field %q", key))
			}
		}
	}
	harnesses := nativeHarnessKeys(rule)
	if len(harnesses) == 0 {
		v.add("native-mapping", source, "native target requires at least one harness hook mapping")
	}
	for _, harness := range harnesses {
		value, _ := nativeMapping(rule, harness)
		mapping := asStringMap(value)
		if mapping == nil {
			v.add("native-mapping", source, fmt.Sprintf("native %s hook must be a mapping", harness))
			continue
		}
		v.validateNativeMapping(harness, mapping, source)
	}
}

func (v *validator) validateNativeMapping(harness string, mapping map[string]any, source string) {
	for _, key := range sortedMapKeys(mapping) {
		if !nativeMappingFields[key] {
			v.add("native-field", source, fmt.Sprintf("native %s hook field %q is not supported", harness, key))
		}
	}
	if event, ok := mapping["event"].(string); !ok || event == "" {
		v.add("native-event", source, fmt.Sprintf("native %s hook requires a string event", harness))
	}
	if value, exists := mapping["matcher"]; exists {
		matcher, ok := value.(string)
		if !ok || strings.TrimSpace(matcher) == "" || matcher != strings.TrimSpace(matcher) || strings.ContainsAny(matcher, "\r\n\x00") {
			v.add("native-matcher", source, fmt.Sprintf("native %s matcher must be a non-empty single-line string without surrounding whitespace", harness))
		}
	}
	command, ok := mapping["command"].(string)
	if !ok {
		v.add("native-command", source, fmt.Sprintf("native %s hook requires a string command", harness))
	} else if problem := nativeCommandProblem(command); problem != "" {
		v.add("native-command", source, fmt.Sprintf("native %s command %s", harness, problem))
	}
	if value, exists := mapping["timeout"]; exists {
		if seconds, ok := integerValue(value); !ok || seconds < 1 || seconds > maxNativeTimeout {
			v.add("native-timeout", source, fmt.Sprintf("native %s timeout must be an integer from 1 to %d seconds", harness, maxNativeTimeout))
		}
	}
}

// nativeCommandProblem enforces the constraints that keep the command's
// meaning unchanged once the ": --owner=…;" ownership prefix is added.
func nativeCommandProblem(command string) string {
	switch {
	case strings.TrimSpace(command) == "":
		return "must not be empty"
	case strings.ContainsAny(command, "\r\n\x00"):
		return "must be a single line without NUL"
	case command != strings.TrimSpace(command):
		return "must not have surrounding whitespace"
	case strings.ContainsRune(";&|)#", rune(command[0])):
		return fmt.Sprintf("must not start with %q", command[0])
	}
	const (
		plain = iota
		single
		double
	)
	state := plain
	for index := 0; index < len(command); index++ {
		switch char := command[index]; state {
		case plain:
			switch char {
			case '\\':
				if index == len(command)-1 {
					return "must not end with an unescaped backslash"
				}
				index++
			case '\'':
				state = single
			case '"':
				state = double
			}
		case single:
			if char == '\'' {
				state = plain
			}
		case double:
			switch char {
			case '\\':
				index++
			case '"':
				state = plain
			}
		}
	}
	if state != plain {
		return "has an unterminated quote"
	}
	return ""
}

func integerValue(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int32:
		return int(number), true
	case int64:
		return int(number), true
	case uint:
		return int(number), true
	case uint32:
		return int(number), true
	case uint64:
		if number > uint64(maxNativeTimeout) {
			return maxNativeTimeout + 1, true
		}
		return int(number), true
	default:
		return 0, false
	}
}

// ValidateNative checks only native rules against the supplied hook surfaces.
// It is a low-level compatibility entry point; Validate plus ValidateNative
// does not check ordinary harness references. Package admission must use
// ValidateWithCatalog.
func ValidateNative(pkg *Package, catalog NativeCatalog) []Diagnostic {
	return validateNative(pkg, catalog, nil)
}

func validateNative(pkg *Package, catalog NativeCatalog, harnesses map[string]bool) []Diagnostic {
	if pkg == nil {
		return nil
	}
	names := catalog.Names()
	if harnesses != nil {
		names = make([]string, 0, len(harnesses))
		for name := range harnesses {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	v := &validator{pkg: pkg}
	for index, rule := range pkg.Rules {
		if !isNativeRule(rule, pkg) {
			continue
		}
		source := rule.Source
		if source == "" {
			source = fmt.Sprintf("rules[%d]", index)
		}
		for _, harness := range nativeHarnessKeys(rule) {
			value, _ := nativeMapping(rule, harness)
			mapping := asStringMap(value)
			surface, registered := catalog[harness]
			known := registered && surface != nil
			if harnesses != nil {
				known = harnesses[harness]
			}
			if !known {
				v.add("native-field", source, fmt.Sprintf("unknown field %q: not a STYLE field or registered harness (registered: %s)", harness, strings.Join(names, ", ")))
				continue
			}
			if surface == nil {
				v.add("native-field", source, fmt.Sprintf("harness %q does not support native hooks", harness))
				continue
			}
			if mapping == nil {
				continue // native-mapping is reported by Validate.
			}
			event, ok := mapping["event"].(string)
			if !ok || event == "" {
				continue
			}
			acceptsMatcher, known := surface.NativeEvent(event)
			if !known {
				v.add("native-event", source, fmt.Sprintf("%s has no native event %q", harness, event))
				continue
			}
			matcher, ok := mapping["matcher"].(string)
			if !ok || strings.TrimSpace(matcher) == "" || matcher != strings.TrimSpace(matcher) {
				continue
			}
			if !acceptsMatcher {
				v.add("native-matcher", source, fmt.Sprintf("%s event %q does not accept a matcher", harness, event))
			} else if !surface.ValidMatcher(matcher) {
				v.add("native-matcher", source, fmt.Sprintf("%s matcher %q is not valid", harness, matcher))
			}
		}
	}
	return v.diagnostics
}

// Names returns the registered harness names in lexical order.
func (catalog NativeCatalog) Names() []string {
	names := make([]string, 0, len(catalog))
	for name := range catalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NativeRules returns the harness mappings of a validated native rule in
// harness order. Mappings that are not well formed are omitted.
func NativeRules(rule Rule) []NativeRule {
	id, _ := rule.Fields["id"].(string)
	var result []NativeRule
	for _, harness := range nativeHarnessKeys(rule) {
		value, _ := nativeMapping(rule, harness)
		mapping := asStringMap(value)
		if mapping == nil {
			continue
		}
		native := NativeRule{RuleID: id, Harness: harness}
		native.Event, _ = mapping["event"].(string)
		native.Matcher, _ = mapping["matcher"].(string)
		native.Command, _ = mapping["command"].(string)
		native.Timeout, _ = integerValue(mapping["timeout"])
		result = append(result, native)
	}
	return result
}
