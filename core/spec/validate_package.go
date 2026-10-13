package spec

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	packageNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	ruleIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_./-]{0,127}$`)
	semverPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
)

var packageFields = stringSet("spec", "name", "version", "description", "authors", "requires", "reads", "vars", "settings", "recommends", "priority", "budget", "defaults", "rules")
var ruleFields = stringSet("id", "target", "file", "text", "model_text", "action", "at", "when", "repeat", "reinject_on_compact", "sources", "fallback", "priority", "max_tokens", "reason", "from", "enabled", "mode", "tool", "names", "mcp", "skill", "event", "reads")
var defaultsFields = stringSet("action", "at", "repeat", "reinject_on_compact", "sources", "fallback", "priority", "max_tokens")

func (v *validator) packageSource() string {
	if v.pkg.SingleFile && len(v.pkg.Rules) > 0 && v.pkg.Rules[0].Source != "" {
		return v.pkg.Rules[0].Source
	}
	return "STYLE.yaml"
}

func (v *validator) validatePackage() {
	manifest := v.pkg.Manifest
	if hasNonStringMapKey(manifest) || hasNonStringMapKey(v.pkg.Metadata) {
		v.add("invalid-schema", v.packageSource(), "YAML mappings must use string keys")
	}
	if v.pkg.SingleFile && hasKey(v.pkg.Metadata, "rules") {
		v.add("invalid-schema", v.packageSource(), "single-file packages contain exactly one rule and cannot declare rules")
	}
	for _, key := range sortedMapKeys(manifest) {
		if !packageFields[key] && (!v.pkg.SingleFile || !ruleFields[key]) {
			v.add("invalid-schema", v.packageSource(), fmt.Sprintf("unknown package field %q", key))
		}
	}
	if got, ok := v.pkg.Metadata["spec"].(string); !ok || got != "style/v0.3" {
		v.add("invalid-spec", v.packageSource(), "spec must be style/v0.3")
	}
	name, ok := v.pkg.Metadata["name"].(string)
	if !ok || !packageNamePattern.MatchString(name) {
		v.add("bad-name", v.packageSource(), "name must match ^[a-z0-9][a-z0-9-]{0,63}$")
	}
	version, ok := v.pkg.Metadata["version"].(string)
	if !ok || !semverPattern.MatchString(version) {
		v.add("bad-version", v.packageSource(), "version must be a semantic version")
	}
	if description, ok := v.pkg.Metadata["description"].(string); !ok || strings.TrimSpace(description) == "" {
		v.add("invalid-schema", v.packageSource(), "description must be a non-empty string")
	}
	if authors, exists := v.pkg.Metadata["authors"]; exists && !isStringOrStringList(authors) {
		v.add("invalid-schema", v.packageSource(), "authors must be a string or list of strings")
	}
	v.validateRequires(v.pkg.Metadata["requires"])
	v.validateVars(v.pkg.Metadata["vars"])
	v.validateSettings(v.pkg.Metadata["settings"])
	v.validateBudget(v.pkg.Metadata["budget"])
	if priority, exists := v.pkg.Metadata["priority"]; exists && !isNumber(priority) {
		v.add("invalid-schema", v.packageSource(), "priority must be a number")
	}
	if defaults, exists := v.pkg.Metadata["defaults"]; exists {
		fields := asStringMap(defaults)
		if fields == nil {
			v.add("invalid-schema", v.packageSource(), "defaults must be a mapping")
		} else {
			for _, key := range sortedMapKeys(fields) {
				if !defaultsFields[key] {
					v.add("defaults-field", v.packageSource(), fmt.Sprintf("defaults cannot set %q", key))
				} else if !validRuleFieldValue(key, fields[key]) {
					v.add("invalid-schema", v.packageSource(), fmt.Sprintf("invalid value type for defaults.%s", key))
				} else if !validRuleFieldEnum(key, fields[key]) {
					v.add("invalid-schema", v.packageSource(), fmt.Sprintf("invalid defaults.%s value %q", key, fields[key]))
				}
			}
		}
	}
	v.validateRecommends(v.pkg.Metadata["recommends"])
	if !v.pkg.SingleFile && len(v.pkg.Rules) == 0 {
		v.add("invalid-schema", v.packageSource(), "rules must contain at least one rule")
	}
}

func (v *validator) validateRequires(value any) {
	source := v.packageSource()
	if value == nil {
		return
	}
	requires := asStringMap(value)
	if requires == nil {
		v.add("invalid-schema", source, "requires must be a mapping")
		return
	}
	for _, key := range sortedMapKeys(requires) {
		if key != "runtime" && key != "harness" {
			v.add("invalid-schema", source, fmt.Sprintf("unknown requires field %q", key))
		}
	}
	if runtime, exists := requires["runtime"]; exists {
		if text, ok := runtime.(string); !ok || !validVersionConstraint(text) {
			v.add("invalid-schema", source, "requires.runtime must be a semantic version constraint")
		}
	}
	if raw, exists := requires["harness"]; exists {
		harness := asStringMap(raw)
		if harness == nil {
			v.add("invalid-schema", source, "requires.harness must be a mapping")
			return
		}
		for _, name := range sortedMapKeys(harness) {
			if name != "codex" && name != "claude-code" {
				v.add("invalid-schema", source, fmt.Sprintf("unknown harness %q", name))
				continue
			}
			if text, ok := harness[name].(string); !ok || !validVersionConstraint(text) {
				v.add("invalid-schema", source, fmt.Sprintf("requires.harness.%s must be a semantic version constraint", name))
			}
		}
	}
}

func validVersionConstraint(value string) bool {
	value = strings.TrimSpace(value)
	for _, operator := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(value, operator) {
			value = strings.TrimSpace(strings.TrimPrefix(value, operator))
			break
		}
	}
	return semverPattern.MatchString(value)
}

func (v *validator) validateVars(value any) {
	source := v.packageSource()
	if value == nil {
		return
	}
	vars := asStringMap(value)
	if vars == nil {
		v.add("invalid-schema", source, "vars must be a mapping")
		return
	}
	for _, key := range sortedMapKeys(vars) {
		if key == "" || strings.ContainsAny(key, ".{}| \t\r\n") || !isScalar(vars[key]) {
			v.add("invalid-schema", source, fmt.Sprintf("vars.%s must have a simple name and scalar value", key))
		}
	}
}

func (v *validator) validateSettings(value any) {
	packageSource := v.packageSource()
	if value == nil {
		return
	}
	settings, ok := value.([]any)
	if !ok {
		v.add("invalid-schema", packageSource, "settings must be a list")
		return
	}
	seen := make(map[string]bool)
	for index, entry := range settings {
		source := fmt.Sprintf("%s:settings[%d]", packageSource, index)
		fields := asStringMap(entry)
		if fields == nil {
			v.add("invalid-schema", source, "setting must be a mapping")
			continue
		}
		for _, key := range sortedMapKeys(fields) {
			if key == "sensitive" {
				v.add("sensitive-reserved", source, "settings.sensitive is reserved in M1")
			} else if key != "name" && key != "description" && key != "default" {
				v.add("invalid-schema", source, fmt.Sprintf("unknown setting field %q", key))
			}
		}
		name, ok := fields["name"].(string)
		if !ok || name == "" {
			v.add("invalid-schema", source, "setting name must be a non-empty string")
			continue
		}
		if seen[name] {
			v.add("invalid-schema", source, fmt.Sprintf("duplicate setting %q", name))
		}
		seen[name] = true
		for _, key := range []string{"description", "default"} {
			if value, exists := fields[key]; exists {
				if _, ok := value.(string); !ok {
					v.add("invalid-schema", source, fmt.Sprintf("setting %s must be a string", key))
				}
			}
		}
	}
}

func (v *validator) validateBudget(value any) {
	source := v.packageSource()
	if value == nil {
		return
	}
	budget := asStringMap(value)
	if budget == nil {
		v.add("invalid-schema", source, "budget must be a mapping")
		return
	}
	for _, key := range sortedMapKeys(budget) {
		switch key {
		case "per_rule", "per_event", "always_on":
			if !isNumber(budget[key]) || toFloat64(budget[key]) <= 0 {
				v.add("invalid-schema", source, fmt.Sprintf("budget.%s must be a positive number", key))
			}
		case "on":
			if text, ok := budget[key].(string); !ok || (text != "warn" && text != "error") {
				v.add("invalid-schema", source, "budget.on must be warn or error")
			}
		default:
			v.add("invalid-schema", source, fmt.Sprintf("unknown budget field %q", key))
		}
	}
}

func (v *validator) validateRecommends(value any) {
	source := v.packageSource()
	if value == nil {
		return
	}
	recommends := asStringMap(value)
	if recommends == nil {
		v.add("invalid-schema", source, "recommends must be a mapping")
		return
	}
	for _, harness := range sortedMapKeys(recommends) {
		if harness != "codex" && harness != "claude-code" {
			v.add("invalid-schema", source, fmt.Sprintf("unknown recommends harness %q", harness))
		}
		if asStringMap(recommends[harness]) == nil {
			v.add("invalid-schema", source, fmt.Sprintf("recommends.%s must be a mapping", harness))
		}
	}
}
