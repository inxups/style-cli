package spec

import (
	"fmt"
	styletemplate "github.com/inxups/style-cli/core/template"
	"strings"
)

var harnessReadFields = stringSet("name", "version", "cwd", "permission_mode", "agent", "session_id", "turn_id")

func (v *validator) collectReads() {
	declarations := []struct {
		value  any
		source string
	}{{v.pkg.Metadata["reads"], v.packageSource()}}
	for _, rule := range v.pkg.Rules {
		declarations = append(declarations,
			struct {
				value  any
				source string
			}{rule.Fields["reads"], rule.Source},
			struct {
				value  any
				source string
			}{rule.Overrides["reads"], rule.Source},
		)
	}
	for _, declaration := range declarations {
		v.validateReads(declaration.value, declaration.source)
		reads := asStringMap(declaration.value)
		if enabled, _ := reads["model"].(bool); enabled {
			v.declaredReads["model"] = true
		}
		if enabled, _ := reads["provider"].(bool); enabled {
			v.declaredReads["provider"] = true
		}
		if enabled, _ := reads["thinking_effort"].(bool); enabled {
			v.declaredReads["thinking_effort"] = true
		}
		for _, field := range stringValues(reads["event"]) {
			v.declaredReads["event."+field] = true
		}
		for _, field := range stringValues(reads["harness"]) {
			v.declaredHarness[field] = true
		}
	}
}

func (v *validator) validateReads(value any, source string) {
	if value == nil {
		return
	}
	reads := asStringMap(value)
	if reads == nil {
		v.add("invalid-schema", source, "reads must be a mapping")
		return
	}
	for _, key := range sortedMapKeys(reads) {
		item := reads[key]
		switch key {
		case "model", "provider", "thinking_effort":
			if enabled, ok := item.(bool); !ok || !enabled {
				v.add("invalid-schema", source, fmt.Sprintf("reads.%s must be true", key))
			}
		case "event":
			values, ok := item.([]any)
			if !ok {
				v.add("invalid-schema", source, "reads.event must be a list")
				continue
			}
			seen := map[string]bool{}
			for _, value := range values {
				name, ok := value.(string)
				if !ok || !styletemplate.EventFieldKnown(name) {
					v.add("invalid-schema", source, fmt.Sprintf("unknown reads.event field %q", value))
				} else if seen[name] {
					v.add("invalid-schema", source, "duplicate reads.event field "+name)
				} else {
					seen[name] = true
				}
			}
		case "harness":
			values, ok := item.([]any)
			if !ok {
				v.add("invalid-schema", source, "reads.harness must be a list")
				continue
			}
			seen := make(map[string]bool)
			for _, value := range values {
				name, ok := value.(string)
				if !ok || !harnessReadFields[name] {
					v.add("invalid-schema", source, fmt.Sprintf("unknown reads.harness field %q", value))
				} else if seen[name] {
					v.add("invalid-schema", source, fmt.Sprintf("duplicate reads.harness field %q", name))
				} else {
					seen[name] = true
				}
			}
		default:
			v.add("invalid-schema", source, fmt.Sprintf("unknown reads field %q", key))
		}
	}
}

func (v *validator) requireRuntimeRead(name, source string) {
	if strings.HasPrefix(name, "event.") {
		v.usedReads[name] = true
		if !v.declaredReads[name] {
			v.add("reads-missing", source, name+" is used but not declared in reads.event")
		}
		return
	}
	if name == "model" || name == "provider" {
		v.usedReads[name] = true
		if !v.declaredReads[name] {
			v.add("reads-missing", source, name+" is used but not declared in reads")
		}
		return
	}
	if name == "thinking_effort" || name == "thinking_effort_raw" {
		v.usedReads["thinking_effort"] = true
		if !v.declaredReads["thinking_effort"] {
			v.add("reads-missing", source, name+" is used but reads.thinking_effort is not declared")
		}
		return
	}
	if len(name) > len("harness.") && name[:len("harness.")] == "harness." {
		field := name[len("harness."):]
		v.usedReads["harness."+field] = true
		if !harnessReadFields[field] || !v.declaredHarness[field] {
			v.add("reads-missing", source, name+" is used but not declared in reads.harness")
		}
	}
}

func (v *validator) reportUnusedReads() {
	declarations := []struct {
		value  any
		source string
	}{{v.pkg.Metadata["reads"], v.packageSource()}}
	for _, rule := range v.pkg.Rules {
		declarations = append(declarations,
			struct {
				value  any
				source string
			}{rule.Fields["reads"], rule.Source},
			struct {
				value  any
				source string
			}{rule.Overrides["reads"], rule.Source},
		)
	}
	for _, declaration := range declarations {
		reads := asStringMap(declaration.value)
		if reads == nil {
			continue
		}
		for _, key := range []string{"model", "provider", "thinking_effort"} {
			if enabled, _ := reads[key].(bool); enabled && !v.usedReads[key] {
				v.warn("reads-unused", declaration.source, "reads."+key+" is declared but unused")
			}
		}
		for _, field := range stringValues(reads["event"]) {
			if !v.usedReads["event."+field] {
				v.warn("reads-unused", declaration.source, "reads.event."+field+" is declared but unused")
			}
		}
		for _, field := range stringValues(reads["harness"]) {
			if !v.usedReads["harness."+field] {
				v.warn("reads-unused", declaration.source, "reads.harness."+field+" is declared but unused")
			}
		}
	}
}
