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
	"unicode/utf8"
)

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
		if !validRuleSource(clean) {
			return nil, parseErr("invalid-schema", clean, "from must reference Markdown files under rules/", nil)
		}
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
	for _, source := range matches {
		if !validRuleSource(source) {
			return nil, parseErr("invalid-schema", source, "from must reference Markdown files under rules/", nil)
		}
	}
	return matches, nil
}

func validRuleSource(source string) bool {
	clean, err := cleanRelativePath(source)
	return err == nil && strings.HasPrefix(clean, "rules/") && strings.HasSuffix(clean, ".md")
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
