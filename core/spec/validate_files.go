package spec

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

func (v *validator) validateUndeclaredRuleFiles() {
	if v.pkg.SingleFile || (v.pkg.Root == "" && v.pkg.inventory == nil) {
		return
	}
	rulesRoot := filepath.Join(v.pkg.Root, "rules")
	declared := make(map[string]bool)
	for _, rule := range v.pkg.Rules {
		declared[filepath.ToSlash(rule.Source)] = true
	}
	var undeclared []string
	if v.pkg.inventory != nil {
		for _, name := range v.pkg.inventory {
			if strings.HasPrefix(name, "rules/") && strings.EqualFold(filepath.Ext(name), ".md") && !declared[name] {
				undeclared = append(undeclared, name)
			}
		}
	} else {
		_ = filepath.WalkDir(rulesRoot, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(v.pkg.Root, name)
			if err == nil && strings.EqualFold(filepath.Ext(entry.Name()), ".md") && !declared[filepath.ToSlash(rel)] {
				undeclared = append(undeclared, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	sort.Strings(undeclared)
	for _, source := range undeclared {
		v.warn("undeclared-rule-file", source, "rule file is not declared by STYLE.yaml and will not be loaded")
	}
}
