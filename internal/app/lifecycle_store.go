package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/spec"
	"gopkg.in/yaml.v3"
)

const (
	styleRuntimeVersion = "0.1.0"
	lockFormat          = 1
)

type lockDocument struct {
	Format    int                    `yaml:"format"`
	Packages  []lockPackage          `yaml:"packages"`
	Harnesses map[string]lockHarness `yaml:"harnesses,omitempty"`
}

type lockPackage struct {
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Seq         int               `yaml:"seq"`
	Source      lockSource        `yaml:"source"`
	Files       map[string]string `yaml:"files"`
	Settings    []string          `yaml:"settings,omitempty"`
	Replace     bool              `yaml:"replace,omitempty"`
	Link        bool              `yaml:"link,omitempty"`
	LinkTrusted bool              `yaml:"link_trusted,omitempty"`
	Approval    string            `yaml:"approval,omitempty"`
}

type lockSource struct {
	Kind string `yaml:"kind"`
	Path string `yaml:"path"`
	Link bool   `yaml:"link"`
}

type lockHarness struct {
	Route      string `yaml:"route"`
	Version    string `yaml:"version,omitempty"`
	ConfigDir  string `yaml:"config_dir,omitempty"`
	Surface    string `yaml:"surface,omitempty"`
	Rendered   string `yaml:"rendered,omitempty"`
	Trust      string `yaml:"trust,omitempty"`
	FailClosed bool   `yaml:"fail_closed,omitempty"`
}

func resolveStyleHome(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	if value := strings.TrimSpace(os.Getenv("STYLE_HOME")); value != "" {
		return filepath.Abs(value)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("style home: %w", err)
	}
	return filepath.Join(home, ".style"), nil
}

func loadLock(home string) (lockDocument, error) {
	path := filepath.Join(home, "style.lock.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return lockDocument{Format: lockFormat, Packages: []lockPackage{}, Harnesses: map[string]lockHarness{}}, nil
	}
	if err != nil {
		return lockDocument{}, fmt.Errorf("read lock: %w", err)
	}
	return decodeLock(data)
}

func decodeLock(data []byte) (lockDocument, error) {
	if data == nil {
		return lockDocument{Format: lockFormat, Packages: []lockPackage{}, Harnesses: map[string]lockHarness{}}, nil
	}
	var document lockDocument
	if err := yaml.Unmarshal(data, &document); err != nil {
		return lockDocument{}, fmt.Errorf("decode lock: %w", err)
	}
	if document.Format == 0 {
		document.Format = lockFormat
	}
	if document.Format != lockFormat {
		return lockDocument{}, fmt.Errorf("unsupported lock format %d", document.Format)
	}
	if document.Packages == nil {
		document.Packages = []lockPackage{}
	}
	if document.Harnesses == nil {
		document.Harnesses = map[string]lockHarness{}
	}
	return document, nil
}

func writeLock(home string, document lockDocument) error {
	document.Format = lockFormat
	sort.Slice(document.Packages, func(i, j int) bool { return document.Packages[i].Seq < document.Packages[j].Seq })
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode lock: %w", err)
	}
	return writeAtomic(filepath.Join(home, "style.lock.yaml"), data, 0o600)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".style-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func snapshotPackage(source string) (map[string][]byte, bool, string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, false, "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, false, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false, "", &spec.ParseError{Code: "symlink", Path: source, Msg: "package source is a symbolic link"}
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() || !strings.HasSuffix(strings.ToLower(info.Name()), ".style.md") {
			return nil, false, "", fmt.Errorf("expected package directory or .style.md file")
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, false, "", err
		}
		if len(data) > spec.MaxFileSize {
			return nil, false, "", &spec.ParseError{Code: "file-too-large", Path: info.Name(), Msg: "package file exceeds size limit"}
		}
		return map[string][]byte{filepath.Base(abs): data}, true, abs, nil
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, false, "", err
	}
	files := make(map[string][]byte)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return &spec.ParseError{Code: "symlink", Path: filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), Msg: "package files cannot be symbolic links"}
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("package path %q is not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) > spec.MaxFileSize {
			return &spec.ParseError{Code: "file-too-large", Path: filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), Msg: "package file exceeds size limit"}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, false, "", err
	}
	return files, false, root, nil
}

func hashPackageFiles(files map[string][]byte) (map[string]string, string) {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, filepath.ToSlash(path))
	}
	sort.Strings(paths)
	result := make(map[string]string, len(paths))
	var digestInput bytes.Buffer
	for _, path := range paths {
		value := files[path]
		fileHash := hashBytes(value)
		result[path] = fileHash
		digestInput.WriteString(path)
		digestInput.WriteByte(0)
		digestInput.WriteString(fileHash)
		digestInput.WriteByte('\n')
	}
	return result, hashBytes(digestInput.Bytes())
}

type packageSwap struct {
	target     string
	backup     string
	backupRoot string
	staged     string
	remove     bool
	committed  bool
	applied    bool
}

func stagePackageFiles(home, name string, single bool, files map[string][]byte) (*packageSwap, error) {
	packages := filepath.Join(home, "packages")
	if err := os.MkdirAll(packages, 0o700); err != nil {
		return nil, err
	}
	if single {
		tmp, err := os.CreateTemp(packages, "."+name+".tmp-*.style.md")
		if err != nil {
			return nil, err
		}
		tmpName := tmp.Name()
		var source []byte
		paths := make([]string, 0, len(files))
		for path := range files {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		if len(paths) > 0 {
			source = files[paths[0]]
		}
		if _, err := tmp.Write(source); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return nil, err
		}
		if err := tmp.Chmod(0o600); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return nil, err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpName)
			return nil, err
		}
		return swapPackage(filepath.Join(packages, name+".style.md"), tmpName), nil
	}
	tmp, err := os.MkdirTemp(packages, "."+name+".tmp-")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, filepath.ToSlash(path))
	}
	sort.Strings(paths)
	for _, rel := range paths {
		path := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := writeAtomic(path, files[rel], 0o600); err != nil {
			os.RemoveAll(tmp)
			return nil, err
		}
	}
	return swapPackage(filepath.Join(packages, name), tmp), nil
}

func swapPackage(destination, staged string) *packageSwap {
	return &packageSwap{target: destination, staged: staged}
}

func stagePackageRemoval(destination string) (*packageSwap, error) {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to remove non-package storage %s", destination)
	}
	return &packageSwap{target: destination, remove: true}, nil
}

func (swap *packageSwap) apply() error {
	if swap == nil {
		return nil
	}
	backupRoot, err := os.MkdirTemp(filepath.Dir(swap.target), ".style-backup-*")
	if err != nil {
		return err
	}
	backup := filepath.Join(backupRoot, "old")
	if err := os.Rename(swap.target, backup); err != nil {
		_ = os.RemoveAll(backupRoot)
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		backupRoot, backup = "", ""
	}
	swap.backupRoot = backupRoot
	swap.backup = backup
	if !swap.remove {
		if err := os.Rename(swap.staged, swap.target); err != nil {
			if swap.backup != "" {
				_ = os.Rename(swap.backup, swap.target)
			}
			if swap.backupRoot != "" {
				_ = os.RemoveAll(swap.backupRoot)
			}
			swap.backup, swap.backupRoot = "", ""
			return err
		}
	}
	swap.applied = true
	return nil
}

func (swap *packageSwap) commit() {
	if swap == nil {
		return
	}
	swap.committed = true
	if swap.backupRoot != "" {
		_ = os.RemoveAll(swap.backupRoot)
	}
}

func (swap *packageSwap) rollback() {
	if swap == nil {
		return
	}
	if swap.committed {
		return
	}
	if !swap.applied {
		_ = os.RemoveAll(swap.staged)
		if swap.backupRoot != "" {
			_ = os.RemoveAll(swap.backupRoot)
		}
		return
	}
	_ = os.RemoveAll(swap.target)
	if swap.backup != "" {
		_ = os.Rename(swap.backup, swap.target)
	}
	if swap.backupRoot != "" {
		_ = os.RemoveAll(swap.backupRoot)
	}
	_ = os.RemoveAll(swap.staged)
}

func readOptionalBytes(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func filesChanged(pkg lockPackage, files map[string][]byte) []string {
	current, _ := hashPackageFiles(files)
	paths := make([]string, 0)
	seen := make(map[string]bool)
	for path, expected := range pkg.Files {
		seen[path] = true
		if current[path] != expected {
			paths = append(paths, path)
		}
	}
	for path := range current {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
