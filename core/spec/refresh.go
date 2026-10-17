package spec

import "strings"

const MaxRefresh = 1000000

// RefreshInterval accepts source integers only; floats and booleans are not
// coerced. The bool distinguishes an explicit zero from an invalid value.
func RefreshInterval(value any) (int, bool) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case uint:
		if uint64(v) > MaxRefresh {
			return 0, false
		}
		n = int64(v)
	case uint32:
		n = int64(v)
	case uint64:
		if v > MaxRefresh {
			return 0, false
		}
		n = int64(v)
	default:
		return 0, false
	}
	return int(n), n >= 0 && n <= MaxRefresh
}

func ruleValueCode(key string) string {
	if key == "refresh" {
		return "refresh-invalid"
	}
	return "invalid-schema"
}

func (v *validator) validateRefresh(rule Rule, source string) {
	value, exists := rule.Overrides["refresh"]
	if !exists {
		value, exists = rule.Fields["refresh"]
	}
	if !exists {
		value = asStringMap(v.pkg.Metadata["defaults"])["refresh"]
	}
	n, valid := RefreshInterval(value)
	if !valid || n == 0 {
		return
	} // Declaration type diagnostics are emitted above.
	action := v.effectiveRuleString(rule, "action", "inject")
	repeatDefault := "session"
	if action == "warn" {
		repeatDefault = "every"
	}
	if (action != "inject" && action != "warn") || v.effectiveRuleString(rule, "repeat", repeatDefault) != "session" ||
		v.effectiveRuleString(rule, "event", "") == "subagent.start" || v.effectiveRuleString(rule, "mode", "append") == "replace" {
		v.add("refresh-incompatible", source, "positive refresh requires inject/warn, repeat session, and a non-replace output point other than subagent.start")
	}
	if action == "inject" && strings.TrimSpace(v.effectiveRuleString(rule, "reason", "")) == "" {
		v.warn("T007", source, "inject with refresh should include a non-empty reason")
	}
}
