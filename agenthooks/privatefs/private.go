// Package privatefs verifies private records using the host security model.
package privatefs

import (
	"errors"
	"os"
)

// CheckRoot binds permission checks to the same regular inode opened beneath root.
func CheckRoot(root *os.Root, name string) error {
	before, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() && !before.Mode().IsRegular() {
		return errors.New("private regular file or directory required")
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, info) {
		return errors.New("private record changed")
	}
	return Check(file)
}

func CheckPath(path string) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return errors.New("private symlink refused")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return errors.New("private record changed")
	}
	return Check(file)
}
