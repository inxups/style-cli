package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var initNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ConflictError reports an existing destination that would be overwritten.
type ConflictError struct {
	Path    string
	Message string
}

func (err *ConflictError) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return fmt.Sprintf("app: init destination already exists: %s", err.Path)
}

// InitRequest describes the package skeleton to create. If Name is omitted,
// a directory package is initialized in OutputDir; single-file packages use
// the basename of OutputDir as their filename and package name.
type InitRequest struct {
	Name      string
	OutputDir string
	Single    bool
}

// InitResult identifies the created package path and format.
type InitResult struct {
	Name   string
	Path   string
	Single bool
}

// Init creates a valid directory package or single-file package without
// replacing any existing file.
func (s *Service) Init(ctx context.Context, request InitRequest) (InitResult, error) {
	if err := checkContext(ctx); err != nil {
		return InitResult{}, err
	}
	dir := request.OutputDir
	if dir == "" {
		dir = "."
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return InitResult{}, fmt.Errorf("app: resolve init directory: %w", err)
	}
	name := strings.TrimSpace(request.Name)
	nameProvided := name != ""
	if name == "" {
		name = defaultPackageName(absDir)
	}
	if !initNamePattern.MatchString(name) {
		return InitResult{}, fmt.Errorf("app: invalid package name %q; use lowercase letters, digits, and hyphens", name)
	}
	if request.Single {
		path := filepath.Join(absDir, name+".style.md")
		content := singleFileSkeleton(name)
		if err := writeNewFile(path, []byte(content), 0o644); err != nil {
			return InitResult{}, err
		}
		return InitResult{Name: name, Path: path, Single: true}, nil
	}
	path := absDir
	if nameProvided {
		path = filepath.Join(absDir, name)
	}
	if err := createDirectoryPackage(path, name, !nameProvided); err != nil {
		return InitResult{}, err
	}
	return InitResult{Name: name, Path: path}, nil
}

func singleFileSkeleton(name string) string {
	return fmt.Sprintf("---\nspec: style/v0.3\nname: %s\nversion: 0.1.0\ndescription: STYLE package %s\ntarget: system\n---\n\nWrite the reusable style instructions here.\n", name, name)
}

func createDirectoryPackage(path, name string, existing bool) error {
	created := make([]string, 0, 5)
	if existing {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("app: inspect init directory %s: %w", path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("app: init destination is not a directory: %s", path)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Errorf("app: read init directory %s: %w", path, err)
		}
		if len(entries) != 0 {
			return &ConflictError{Path: path}
		}
	} else {
		if _, err := os.Lstat(path); err == nil {
			return &ConflictError{Path: path}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("app: inspect init destination %s: %w", path, err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("app: create package directory %s: %w", path, err)
		}
		created = append(created, path)
	}
	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
	}
	for _, subdir := range []string{"prompts", "rules"} {
		directory := filepath.Join(path, subdir)
		if err := os.Mkdir(directory, 0o755); err != nil {
			if !errors.Is(err, os.ErrExist) {
				rollback()
				return fmt.Errorf("app: create %s: %w", subdir, err)
			}
			info, statErr := os.Stat(directory)
			if statErr != nil || !info.IsDir() {
				rollback()
				return fmt.Errorf("app: init path %s is not a directory", directory)
			}
			continue
		}
		created = append(created, directory)
	}
	files := []struct {
		name string
		body string
	}{
		{"STYLE.yaml", directoryManifest(name)},
		{"prompts/default.md", "Write the reusable style instructions here.\n"},
		{"rules/review.md", "---\ntarget: tool\ntool: shell\naction: inject\nat: after\n---\nReview shell results according to this package's style.\n"},
	}
	for _, file := range files {
		full := filepath.Join(path, filepath.FromSlash(file.name))
		if err := writeNewFile(full, []byte(file.body), 0o644); err != nil {
			rollback()
			return err
		}
		created = append(created, full)
	}
	return nil
}

func defaultPackageName(directory string) string {
	base := strings.ToLower(filepath.Base(directory))
	var name strings.Builder
	separator := false
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			name.WriteRune(r)
			separator = false
			continue
		}
		if unicode.IsSpace(r) || r == '_' || r == '-' {
			if name.Len() > 0 && !separator {
				name.WriteByte('-')
				separator = true
			}
		}
	}
	value := strings.Trim(name.String(), "-")
	if len(value) > 64 {
		value = strings.TrimRight(value[:64], "-")
	}
	if value == "" {
		return "style"
	}
	return value
}

func directoryManifest(name string) string {
	return fmt.Sprintf("spec: style/v0.3\nname: %s\nversion: 0.1.0\ndescription: STYLE package %s\nrules:\n  - id: style\n    target: system\n    file: prompts/default.md\n  - from: rules/review.md\n", name, name)
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return &ConflictError{Path: path}
		}
		return fmt.Errorf("app: create %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("app: write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("app: close %s: %w", path, err)
	}
	return nil
}
