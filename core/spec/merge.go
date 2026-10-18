package spec

// Merge applies standard, package, rule-file, and from-entry values in
// increasing precedence.
// Disabled rules are omitted; Package remains available for validation of the
// original declarations, including disabled rules.
func Merge(pkg *Package) (*ResolvedPackage, error) {
	if pkg == nil {
		return nil, parseErr("invalid-package", "", "cannot merge a nil package", nil)
	}
	metadata := pkg.Metadata
	packageDefaults := asStringMap(metadata["defaults"])
	packagePriority, hasPackagePriority := metadata["priority"]
	if !hasPackagePriority {
		packagePriority = 50
	}
	packageMaxTokens := any(400)
	if budget := asStringMap(metadata["budget"]); budget != nil {
		if value, ok := budget["per_rule"]; ok {
			packageMaxTokens = value
		}
	}

	resolved := &ResolvedPackage{Package: pkg}
	for _, original := range pkg.Rules {
		if err := rejectRemovedNative(original.Fields, original.Source); err != nil {
			return nil, err
		}
		effective := map[string]any{
			"action":     "inject",
			"fallback":   "degrade",
			"enabled":    true,
			"priority":   packagePriority,
			"max_tokens": packageMaxTokens,
			"sources":    []any{"startup", "clear", "fork"},
		}
		for key, value := range packageDefaults {
			effective[key] = cloneValue(value)
		}
		for key, value := range original.Fields {
			if key != "from" {
				effective[key] = cloneValue(value)
			}
		}
		for key, value := range original.Overrides {
			effective[key] = cloneValue(value)
		}
		if _, ok := effective["at"]; !ok {
			if effective["action"] == "block" {
				effective["at"] = "before"
			} else {
				effective["at"] = "after"
			}
		}
		if _, ok := effective["reinject_on_compact"]; !ok {
			effective["reinject_on_compact"] = effective["target"] == "system"
		}
		if effective["target"] == "system" {
			if _, ok := effective["mode"]; !ok {
				effective["mode"] = "append"
			}
		}
		if action, _ := effective["action"].(string); action == "block" {
			// Block decisions are deliberately never deduplicated. Preserve any
			// source declaration in Fields so validation can still warn about it.
			delete(effective, "repeat")
		} else if _, ok := effective["repeat"]; !ok {
			if action == "warn" {
				effective["repeat"] = "every"
			} else {
				effective["repeat"] = "session"
			}
		}

		if enabled, isBool := effective["enabled"].(bool); isBool && !enabled {
			continue
		}
		mergedRule := original
		mergedRule.Fields = cloneMap(original.Fields)
		mergedRule.Overrides = cloneMap(original.Overrides)
		mergedRule.Effective = effective
		resolved.Rules = append(resolved.Rules, mergedRule)
	}
	for _, original := range pkg.NativeHooks {
		effective := map[string]any{"enabled": true, "priority": packagePriority, "fallback": "skip"}
		for key, value := range original.Fields {
			effective[key] = cloneValue(value)
		}
		if effective["enabled"] == false {
			continue
		}
		hook := original
		hook.Fields = cloneMap(original.Fields)
		hook.Effective = effective
		resolved.NativeHooks = append(resolved.NativeHooks, hook)
	}
	return resolved, nil
}
