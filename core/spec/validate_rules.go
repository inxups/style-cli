package spec

import (
	"fmt"
	"path/filepath"
	"strings"
)

func (v *validator) validateRules() {
	ids := make(map[string]string)
	for index, rule := range v.pkg.Rules {
		source := rule.Source
		if source == "" {
			source = fmt.Sprintf("rules[%d]", index)
		}
		v.validateRuleFields(rule, source)
		if id, ok := rule.Fields["id"].(string); ok {
			if !validRuleID(id) {
				v.add("bad-id", source, "rule id is not a valid package-relative identifier")
			}
			if first, exists := ids[id]; exists {
				v.add("dup-id", source, fmt.Sprintf("rule id %q duplicates %s", id, first))
			} else {
				ids[id] = source
			}
		} else if _, exists := rule.Fields["id"]; exists {
			v.add("bad-id", source, "rule id must be a string")
		}
		bodySource := rule.BodySource
		if bodySource == "" {
			bodySource = source
		}
		if strings.TrimSpace(rule.Body) == "" {
			v.add("empty-body", bodySource, "rule body must not be empty")
		}
		if strings.Contains(rule.Body, "<!-- style:") {
			v.add("marker-in-body", bodySource, "body contains a reserved STYLE marker")
		}
		v.validateTemplate(rule.Body, bodySource, rule)
		modelText, modelTextExists := rule.Fields["model_text"]
		if override, exists := rule.Overrides["model_text"]; exists {
			modelText, modelTextExists = override, true
		}
		if modelTextExists {
			if text, ok := modelText.(string); ok {
				v.validateTemplate(text, source+"#model_text", rule)
			} else {
				v.add("invalid-schema", source, "model_text must be a string")
			}
		}
		v.validateLint(rule, source)
	}
}

func (v *validator) validateRuleFields(rule Rule, source string) {
	fields := rule.Fields
	isRuleFile := strings.HasPrefix(filepath.ToSlash(source), "rules/")
	for _, key := range sortedMapKeys(fields) {
		if !ruleFields[key] && !(fields["target"] == "native" && (key == "codex" || key == "claude-code")) {
			v.add("invalid-schema", source, fmt.Sprintf("unknown rule field %q", key))
		}
		if isRuleFile && isPackageField(key) && key != "priority" && key != "reads" {
			v.add("rule-file-pkg-field", source, fmt.Sprintf("rule files cannot set package field %q", key))
		}
	}
	if rule.From != "" {
		if !strings.HasPrefix(filepath.ToSlash(rule.Source), "rules/") {
			v.add("invalid-schema", source, "from must reference a file under rules/")
		}
		for _, forbidden := range []string{"id", "file", "text"} {
			if _, exists := rule.Overrides[forbidden]; exists {
				v.add("from-and-inline", source, fmt.Sprintf("from cannot be combined with %s", forbidden))
			}
		}
		for _, protected := range []string{"target", "id"} {
			if _, exists := rule.Overrides[protected]; exists {
				v.add("override-target", source, fmt.Sprintf("from cannot override %s", protected))
			}
		}
	} else if !isRuleFile && !v.pkg.SingleFile {
		if _, exists := fields["id"]; !exists {
			v.add("inline-missing-id", source, "inline rules require id")
		}
		_, hasFile := fields["file"]
		_, hasText := fields["text"]
		if hasFile && hasText {
			v.add("file-and-text", source, "file and text are mutually exclusive")
		} else if !hasFile && !hasText {
			v.add("invalid-schema", source, "inline rules require file or text")
		}
	}
	for _, key := range sortedMapKeys(rule.Overrides) {
		if !ruleFields[key] {
			v.add("invalid-schema", source, fmt.Sprintf("unknown from override field %q", key))
		}
		if !validRuleFieldValue(key, rule.Overrides[key]) {
			v.add("invalid-schema", source, fmt.Sprintf("invalid value type for from override %s", key))
		}
	}
	for _, key := range sortedMapKeys(fields) {
		value := fields[key]
		if !validRuleFieldValue(key, value) {
			v.add("invalid-schema", source, fmt.Sprintf("invalid value type for rule field %s", key))
		}
		switch key {
		case "when":
			if _, overridden := rule.Overrides["when"]; !overridden {
				v.validateWhen(value, rule, source)
			}
		case "reads", "codex", "claude-code":
			// Validated elsewhere or harness-specific native data.
		}
	}
	defaults := asStringMap(v.pkg.Metadata["defaults"])
	for _, key := range []string{"action", "target", "at", "repeat", "fallback", "mode"} {
		value := v.effectiveRuleString(rule, key, "")
		if value == "" {
			continue
		}
		valid := true
		switch key {
		case "action":
			valid = validRuleFieldEnum(key, value)
		case "target":
			valid = stringSet("system", "tool", "mcp", "skill", "event", "native")[value]
		case "at":
			valid = validRuleFieldEnum(key, value)
		case "repeat":
			valid = validRuleFieldEnum(key, value)
		case "fallback":
			valid = validRuleFieldEnum(key, value)
		case "mode":
			valid = stringSet("append", "replace")[value]
		}
		if !valid {
			v.add("invalid-schema", source, fmt.Sprintf("invalid %s value %q", key, value))
		}
	}
	if when, exists := rule.Overrides["when"]; exists {
		v.validateWhen(when, rule, source)
	}
	if target, exists := fields["target"]; !exists || target == "" {
		v.add("invalid-schema", source, "rule target is required")
	}
	if isRuleFile && (hasKey(fields, "file") || hasKey(fields, "text")) {
		v.add("invalid-schema", source, "rule files use their markdown body and cannot set file or text")
	}
	if v.pkg.SingleFile && (hasKey(fields, "from") || hasKey(fields, "file") || hasKey(fields, "text")) {
		v.add("invalid-schema", source, "single-file packages cannot set from, file, or text")
	}
	if target := v.effectiveRuleString(rule, "target", ""); target == "event" {
		event := v.effectiveRuleString(rule, "event", "")
		if !validEvents[event] {
			v.add("invalid-schema", source, "event target requires a supported event")
		}
	}
	if target := v.effectiveRuleString(rule, "target", ""); target == "tool" && !ruleHasField(rule, "tool") && !ruleHasField(rule, "names") {
		v.add("invalid-schema", source, "tool target requires tool or names")
	}
	if target := v.effectiveRuleString(rule, "target", ""); target == "mcp" && !ruleHasField(rule, "mcp") {
		v.add("invalid-schema", source, "mcp target requires mcp")
	}
	if target := v.effectiveRuleString(rule, "target", ""); target == "skill" && !ruleHasField(rule, "skill") {
		v.add("invalid-schema", source, "skill target requires skill")
	}
	if target := v.effectiveRuleString(rule, "target", ""); target == "native" && !ruleHasField(rule, "codex") && !ruleHasField(rule, "claude-code") {
		v.add("invalid-schema", source, "native target requires a harness hook mapping")
	}
	if isRuleFile && hasKey(fields, "from") {
		v.add("invalid-schema", source, "from is only valid in STYLE.yaml")
	}
	if event := v.effectiveRuleString(rule, "event", ""); event != "" && !validEvents[event] {
		v.add("invalid-schema", source, fmt.Sprintf("unknown event %q", event))
	}
	if action := v.effectiveRuleString(rule, "action", "inject"); action == "block" {
		target := v.effectiveRuleString(rule, "target", "")
		if !blockTargetAllowed(target, v.effectiveRuleString(rule, "event", "")) {
			v.add("action-target", source, "block is only valid for tool, mcp, skill, tool.pre, or prompt.submitted")
		}
		if v.effectiveRuleString(rule, "at", "before") == "after" {
			v.add("block-after", source, "block cannot run at after")
		}
	}
	if v.effectiveRuleString(rule, "mode", "") == "replace" && v.effectiveRuleString(rule, "target", "") != "system" {
		v.add("replace-non-system", source, "mode replace is only valid for target system")
	}
	if v.effectiveRuleString(rule, "action", "inject") != "warn" && (hasKey(fields, "model_text") || hasKey(rule.Overrides, "model_text")) {
		v.warn("model-text-unused", source, "model_text is only used by warn rules")
	}
	hasSources := hasKey(fields, "sources") || hasKey(rule.Overrides, "sources") || hasKey(defaults, "sources")
	if hasSources && v.effectiveRuleString(rule, "target", "") != "system" && v.effectiveRuleString(rule, "event", "") != "session.start" {
		v.warn("sources-unused", source, "sources only applies to session.start rules")
	}
	if enabled, _ := v.effectiveRuleBool(rule, "reinject_on_compact"); enabled && v.effectiveRuleString(rule, "repeat", "session") != "session" {
		v.warn("reinject-unused", source, "reinject_on_compact only applies to repeat: session")
	}
	if v.effectiveRuleString(rule, "action", "inject") == "block" && (hasKey(fields, "repeat") || hasKey(rule.Overrides, "repeat") || hasKey(defaults, "repeat")) {
		v.warn("block-repeat", source, "block rules are never deduplicated; repeat is ignored")
	}
	repeat := v.effectiveRuleString(rule, "repeat", "session")
	if v.effectiveRuleString(rule, "action", "inject") == "inject" && (repeat == "every" || repeat == "turn") && !hasKey(fields, "reason") && !hasKey(rule.Overrides, "reason") {
		v.warn("T007", source, "inject with repeat every or turn should include reason")
	}
}

func validRuleFieldValue(key string, value any) bool {
	switch key {
	case "id", "target", "file", "text", "model_text", "action", "at", "repeat", "fallback", "mode", "reason", "from", "event":
		_, ok := value.(string)
		return ok
	case "tool", "names", "mcp", "skill", "sources":
		return isStringOrStringList(value)
	case "enabled", "reinject_on_compact":
		_, ok := value.(bool)
		return ok
	case "priority", "max_tokens":
		return isNumber(value) && toFloat64(value) >= 0
	case "reads", "when", "codex", "claude-code":
		return asStringMap(value) != nil
	default:
		return false
	}
}

func validRuleFieldEnum(key string, value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	switch key {
	case "action":
		return stringSet("inject", "warn", "block")[text]
	case "at":
		return stringSet("before", "after")[text]
	case "repeat":
		return stringSet("every", "turn", "session")[text]
	case "fallback":
		return stringSet("degrade", "skip", "error")[text]
	default:
		return true
	}
}

func (v *validator) effectiveRuleString(rule Rule, key, fallback string) string {
	if value, ok := rule.Overrides[key].(string); ok {
		return value
	}
	if value, ok := rule.Fields[key].(string); ok {
		return value
	}
	if defaults := asStringMap(v.pkg.Metadata["defaults"]); defaults != nil {
		if value, ok := defaults[key].(string); ok {
			return value
		}
	}
	return fallback
}

func (v *validator) effectiveRuleStringValues(rule Rule, key string) []string {
	if value, ok := rule.Overrides[key]; ok {
		return stringValues(value)
	}
	if value, ok := rule.Fields[key]; ok {
		return stringValues(value)
	}
	if defaults := asStringMap(v.pkg.Metadata["defaults"]); defaults != nil {
		return stringValues(defaults[key])
	}
	return nil
}

func (v *validator) effectiveRuleBool(rule Rule, key string) (bool, bool) {
	if value, ok := rule.Overrides[key].(bool); ok {
		return value, true
	}
	if value, ok := rule.Fields[key].(bool); ok {
		return value, true
	}
	if defaults := asStringMap(v.pkg.Metadata["defaults"]); defaults != nil {
		value, ok := defaults[key].(bool)
		return value, ok
	}
	return false, false
}

func ruleHasField(rule Rule, key string) bool {
	return hasKey(rule.Overrides, key) || hasKey(rule.Fields, key)
}

func blockTargetAllowed(target, event string) bool {
	if target == "tool" || target == "mcp" || target == "skill" {
		return true
	}
	return target == "event" && (event == "tool.pre" || event == "prompt.submitted")
}

func validRuleID(id string) bool {
	return ruleIDPattern.MatchString(id) && !strings.Contains(id, "..") && !strings.Contains(id, "//") && !strings.HasPrefix(id, "/") && !strings.HasSuffix(id, "/")
}
