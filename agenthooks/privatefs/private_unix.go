//go:build !windows

package privatefs

import (
	"errors"
	"os"
)

func Check(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() && info.Mode().Perm()&0o077 == 0 || info.Mode().IsRegular() && info.Mode().Perm() == 0o600 {
		return nil
	}
	return errors.New("private permissions required")
}
func Restrict(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info.IsDir() {
		mode = 0o700
	}
	return file.Chmod(mode)
}

// Prepare and PrepareDirectory add Windows DACLs; POSIX creation modes suffice.
func Prepare(file *os.File) error        { return nil }
func PrepareDirectory(path string) error { return nil }
