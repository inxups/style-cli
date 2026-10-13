// Package spec loads STYLE directory and single-file packages.
package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const MaxFileSize = 256 * 1024

// Package is the parsed, unmerged representation of a STYLE package.
// Manifest and Rule.Fields retain source values so later validation and merge
// stages can distinguish declared fields from defaults.
type Package struct {
	Root       string
	Manifest   map[string]any
	Metadata   map[string]any
	Rules      []Rule
	Files      map[string][]byte
	SingleFile bool
}

// Rule is one entry in rules[], or the sole rule in a single-file package.
type Rule struct {
	Index      int
	Source     string
	From       string
	Fields     map[string]any
	Overrides  map[string]any
	Body       string
	BodySource string
}

// ParseError carries a stable machine-readable code for CLI diagnostics.
type ParseError struct {
	Code string
	Path string
	Msg  string
	Err  error
}

func (e *ParseError) Error() string {
	where := ""
	if e.Path != "" {
		where = e.Path + ": "
	}
	if e.Err != nil {
		return where + e.Msg + ": " + e.Err.Error()
	}
	return where + e.Msg
}

func (e *ParseError) Unwrap() error { return e.Err }

// ErrorCode returns the parse or validation code embedded in err.
func ErrorCode(err error) string {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return parseErr.Code
	}
	return ""
}

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

func parseYAMLMap(data []byte, source string) (map[string]any, error) {
	var fields map[string]any
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, parseErr("invalid-yaml", source, "parse YAML", err)
	}
	if fields == nil {
		return nil, parseErr("invalid-yaml", source, "expected a YAML mapping", nil)
	}
	return fields, nil
}

func splitFrontmatter(data []byte, source string) ([]byte, string, error) {
	text := string(data)
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, "", parseErr("bad-encoding", source, "file must be UTF-8 text without NUL bytes", nil)
	}
	start := 0
	if strings.HasPrefix(text, "\ufeff") {
		start = len("\ufeff")
	}
	first, next := nextLine(text, start)
	if trimLineEnding(first) != "---" {
		return nil, "", parseErr("invalid-frontmatter", source, "expected opening --- delimiter", nil)
	}
	yamlStart := next
	lineStart := next
	for lineStart <= len(text) {
		line, after := nextLine(text, lineStart)
		if trimLineEnding(line) == "---" {
			yamlBytes := []byte(text[yamlStart:lineStart])
			body := text[after:]
			body = trimLeadingBlankLines(body)
			body = strings.TrimRightFunc(body, unicode.IsSpace)
			return yamlBytes, body, nil
		}
		if after == lineStart || after >= len(text) {
			break
		}
		lineStart = after
	}
	return nil, "", parseErr("invalid-frontmatter", source, "missing closing --- delimiter", nil)
}

func hasFrontmatter(data []byte) bool {
	text := string(data)
	start := 0
	if strings.HasPrefix(text, "\ufeff") {
		start = len("\ufeff")
	}
	first, _ := nextLine(text, start)
	return trimLineEnding(first) == "---"
}

func nextLine(text string, start int) (string, int) {
	if start >= len(text) {
		return "", len(text)
	}
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		return text[start:], len(text)
	}
	end += start
	return text[start:end], end + 1
}

func trimLineEnding(line string) string { return strings.TrimSuffix(line, "\r") }

func trimLeadingBlankLines(body string) string {
	for body != "" {
		line, after := nextLine(body, 0)
		if strings.TrimSpace(trimLineEnding(line)) != "" {
			return body
		}
		if after == len(body) {
			return ""
		}
		body = body[after:]
	}
	return body
}

func readPackageFile(root, rel string, files map[string][]byte) ([]byte, error) {
	clean, err := cleanRelativePath(rel)
	if err != nil {
		return nil, err
	}
	if data, ok := files[clean]; ok {
		return data, nil
	}
	current := root
	for _, part := range strings.Split(clean, "/") {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, parseErr("file-not-found", clean, "read package file", err)
			}
			return nil, parseErr("file-read", clean, "inspect package file", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, parseErr("symlink", clean, "package files cannot be symbolic links", nil)
		}
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(clean)))
	if err != nil {
		return nil, parseErr("file-read", clean, "inspect package file", err)
	}
	if !info.Mode().IsRegular() {
		return nil, parseErr("file-read", clean, "package path is not a regular file", nil)
	}
	if info.Size() > MaxFileSize {
		return nil, parseErr("file-too-large", clean, fmt.Sprintf("file exceeds %d bytes", MaxFileSize), nil)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(clean)))
	if err != nil {
		return nil, parseErr("file-read", clean, "read package file", err)
	}
	if len(data) > MaxFileSize {
		return nil, parseErr("file-too-large", clean, fmt.Sprintf("file exceeds %d bytes", MaxFileSize), nil)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, parseErr("bad-encoding", clean, "file must be UTF-8 text without NUL bytes", nil)
	}
	files[clean] = data
	return data, nil
}

func cleanRelativePath(rel string) (string, error) {
	if rel == "" || strings.ContainsRune(rel, '\\') || strings.HasPrefix(rel, "/") || filepath.VolumeName(filepath.FromSlash(rel)) != "" || (len(rel) >= 2 && ((rel[0] >= 'a' && rel[0] <= 'z') || (rel[0] >= 'A' && rel[0] <= 'Z')) && rel[1] == ':') {
		return "", parseErr("path-escape", rel, "expected a relative package path", nil)
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", parseErr("path-escape", rel, "parent path segments are not allowed", nil)
		}
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", parseErr("path-escape", rel, "path escapes the package root", nil)
	}
	return clean, nil
}

func expandFrom(root, pattern string) ([]string, error) {
	pattern = filepath.ToSlash(pattern)
	clean, err := cleanRelativePath(pattern)
	if err != nil {
		return nil, err
	}
	if !strings.ContainsAny(clean, "*?[") {
		return []string{clean}, nil
	}
	if err := validateGlob(clean); err != nil {
		return nil, parseErr("bad-glob", clean, "invalid from glob", err)
	}
	var matches []string
	err = filepath.WalkDir(root, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if full == root || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		matched, err := matchGlob(clean, rel)
		if err != nil {
			return err
		}
		if matched {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return nil, parseErr("file-read", clean, "expand from glob", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, parseErr("file-not-found", clean, "from glob matched no files", nil)
	}
	return matches, nil
}

func validateGlob(pattern string) error {
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return err
		}
	}
	return nil
}

func matchGlob(pattern, name string) (bool, error) {
	patterns := strings.Split(pattern, "/")
	parts := strings.Split(name, "/")
	var match func(int, int) (bool, error)
	match = func(pi, ni int) (bool, error) {
		if pi == len(patterns) {
			return ni == len(parts), nil
		}
		if patterns[pi] == "**" {
			for next := ni; next <= len(parts); next++ {
				ok, err := match(pi+1, next)
				if err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		}
		if ni == len(parts) {
			return false, nil
		}
		ok, err := path.Match(patterns[pi], parts[ni])
		if err != nil || !ok {
			return false, err
		}
		return match(pi+1, ni+1)
	}
	return match(0, 0)
}

func cloneMap(fields map[string]any) map[string]any {
	cloned := make(map[string]any, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

func cloneMapWithout(fields map[string]any, omitted string) map[string]any {
	cloned := make(map[string]any, len(fields))
	for key, value := range fields {
		if key != omitted {
			cloned[key] = value
		}
	}
	return cloned
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

func parseErr(code, source, message string, err error) error {
	return &ParseError{Code: code, Path: source, Msg: message, Err: err}
}
