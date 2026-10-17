// Package saferead captures bounded regular files beneath an explicit anchor.
package saferead

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrChanged = errors.New("input changed while capturing")

// Anchor canonicalizes existing parent aliases (such as macOS /tmp), but
// refuses a symlink at the explicitly selected directory itself.
func Anchor(path string) (*os.Root, error) {
	return anchor(path, false)
}

// AnchorMutable retains directory identity and mode while allowing child
// creation/removal to change its size and mtime, as concurrent probe leases do.
// File captures still require the full Same check on the file itself.
func AnchorMutable(path string) (*os.Root, error) {
	return anchor(path, true)
}

func anchor(path string, mutable bool) (*os.Root, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("regular directory required")
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, err
	}
	opened, e := root.Stat(".")
	stable := e == nil && Same(info, opened)
	if mutable && e == nil {
		stable = os.SameFile(info, opened) && info.Mode() == opened.Mode()
	}
	if !stable {
		root.Close()
		return nil, ErrChanged
	}
	return root, nil
}

// CheckDirectories refuses symlinks even if their destination is inside root.
func CheckDirectories(root *os.Root, name string) error {
	parent := filepath.Dir(name)
	if parent == "." {
		return nil
	}
	parts := strings.Split(filepath.ToSlash(parent), "/")
	current := ""
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("regular directory required")
		}
	}
	return nil
}

func Same(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

func File(root *os.Root, name string, limit int64) ([]byte, error) {
	if err := CheckDirectories(root, name); err != nil {
		return nil, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, fmt.Errorf("bounded regular file required")
	}
	f, err := openRegular(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !Same(before, opened) {
		return nil, ErrChanged
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input exceeds limit")
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	pathAfter, err := root.Lstat(name)
	if err != nil {
		return nil, ErrChanged
	}
	if !Same(before, after) || !Same(before, pathAfter) {
		return nil, ErrChanged
	}
	return data, nil
}

func ReadFile(path string, limit int64) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	root, err := Anchor(parent)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return File(root, filepath.Base(absolute), limit)
}
