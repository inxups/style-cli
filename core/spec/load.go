package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load parses either a package directory containing STYLE.yaml or a
// single-file package whose name ends in .style.md.
func Load(source string) (*Package, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, parseErr("invalid-package", source, "resolve package path", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, parseErr("invalid-package", source, "open package", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, parseErr("symlink", source, "package source is a symbolic link", nil)
	}
	if info.IsDir() {
		root, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, parseErr("invalid-package", source, "resolve package directory", err)
		}
		return loadDirectory(root)
	}
	if !info.Mode().IsRegular() || !strings.HasSuffix(strings.ToLower(info.Name()), ".style.md") {
		return nil, parseErr("invalid-package", source, "expected a package directory or .style.md file", nil)
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return nil, parseErr("invalid-package", source, "resolve package directory", err)
	}
	return loadSingleFile(root, filepath.Base(abs))
}

func loadDirectory(root string) (*Package, error) {
	pkg := &Package{Root: root, Files: make(map[string][]byte)}
	manifestBytes, err := readPackageFile(root, "STYLE.yaml", pkg.Files)
	if err != nil {
		return nil, err
	}
	manifest, err := parseYAMLMap(manifestBytes, "STYLE.yaml")
	if err != nil {
		return nil, err
	}
	pkg.Manifest = manifest
	pkg.Metadata = cloneMapWithout(manifest, "rules")
	rulesValue, ok := manifest["rules"]
	if !ok {
		return nil, parseErr("invalid-package", "STYLE.yaml", "missing rules list", nil)
	}
	rules, ok := rulesValue.([]any)
	if !ok {
		return nil, parseErr("invalid-package", "STYLE.yaml", "rules must be a list", nil)
	}
	for index, rawRule := range rules {
		fields, ok := rawRule.(map[string]any)
		if !ok {
			return nil, parseErr("invalid-package", "STYLE.yaml", fmt.Sprintf("rules[%d] must be a mapping", index), nil)
		}
		if fromValue, hasFrom := fields["from"]; hasFrom {
			pattern, ok := fromValue.(string)
			if !ok || strings.TrimSpace(pattern) == "" {
				return nil, parseErr("invalid-package", "STYLE.yaml", fmt.Sprintf("rules[%d].from must be a non-empty path", index), nil)
			}
			matches, err := expandFrom(root, pattern)
			if err != nil {
				return nil, err
			}
			overrides := cloneMapWithout(fields, "from")
			for _, rel := range matches {
				data, err := readPackageFile(root, rel, pkg.Files)
				if err != nil {
					return nil, err
				}
				frontmatter, body, err := splitFrontmatter(data, rel)
				if err != nil {
					return nil, err
				}
				fileFields, err := parseYAMLMap(frontmatter, rel)
				if err != nil {
					return nil, err
				}
				if _, declared := fileFields["id"]; !declared {
					fileFields["id"] = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(rel), "rules/"), ".md")
				}
				pkg.Rules = append(pkg.Rules, Rule{
					Index:      len(pkg.Rules),
					Source:     filepath.ToSlash(rel),
					From:       pattern,
					Fields:     fileFields,
					Overrides:  cloneMap(overrides),
					Body:       body,
					BodySource: filepath.ToSlash(rel),
				})
			}
			continue
		}
		rule := Rule{
			Index:     len(pkg.Rules),
			Source:    inlineSource(fields, len(pkg.Rules)),
			Fields:    cloneMap(fields),
			Overrides: make(map[string]any),
		}
		if textValue, ok := fields["text"].(string); ok {
			rule.Body = textValue
			rule.BodySource = "STYLE.yaml#" + ruleID(fields, rule.Index)
		}
		if fileValue, ok := fields["file"].(string); ok {
			cleanFile, err := cleanRelativePath(fileValue)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(cleanFile, "rules/") {
				return nil, parseErr("file-into-rules", cleanFile, "file references cannot point into rules/", nil)
			}
			body, err := readPackageFile(root, cleanFile, pkg.Files)
			if err != nil {
				return nil, err
			}
			if hasFrontmatter(body) {
				return nil, parseErr("body-has-frontmatter", cleanFile, "body files cannot have frontmatter", nil)
			}
			rule.Body = string(body)
			rule.BodySource = filepath.ToSlash(cleanFile)
		}
		pkg.Rules = append(pkg.Rules, rule)
	}
	return pkg, nil
}

func loadSingleFile(root, name string) (*Package, error) {
	files := make(map[string][]byte)
	data, err := readPackageFile(root, name, files)
	if err != nil {
		return nil, err
	}
	frontmatter, body, err := splitFrontmatter(data, name)
	if err != nil {
		return nil, err
	}
	fields, err := parseYAMLMap(frontmatter, name)
	if err != nil {
		return nil, err
	}
	metadata := make(map[string]any)
	ruleFields := make(map[string]any)
	for key, value := range fields {
		if isPackageField(key) {
			metadata[key] = value
		}
		if !isPackageField(key) || key == "priority" {
			ruleFields[key] = value
		}
	}
	if _, ok := ruleFields["id"]; !ok {
		if packageName, ok := metadata["name"].(string); ok && packageName != "" {
			ruleFields["id"] = packageName
		}
	}
	pkg := &Package{
		Root:       root,
		Manifest:   fields,
		Metadata:   metadata,
		Files:      files,
		SingleFile: true,
		Rules: []Rule{{
			Index:      0,
			Source:     filepath.ToSlash(name),
			Fields:     ruleFields,
			Overrides:  make(map[string]any),
			Body:       body,
			BodySource: filepath.ToSlash(name),
		}},
	}
	return pkg, nil
}

func isPackageField(key string) bool {
	switch key {
	case "spec", "name", "version", "description", "authors", "requires", "reads", "vars", "settings", "recommends", "budget", "defaults", "rules", "priority":
		return true
	default:
		return false
	}
}

func ruleID(fields map[string]any, index int) string {
	if id, ok := fields["id"].(string); ok && id != "" {
		return id
	}
	return fmt.Sprintf("rule-%d", index+1)
}

func inlineSource(fields map[string]any, index int) string {
	return "STYLE.yaml#" + ruleID(fields, index)
}
