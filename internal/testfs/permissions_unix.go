//go:build !windows

// Package testfs supplies real permission faults to tests only.
package testfs

import (
	"os"
	"testing"
)

func WritableDirectory(t testing.TB, path string) func(bool) {
	t.Helper()
	return func(writable bool) {
		mode := os.FileMode(0500)
		if writable {
			mode = 0700
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func ReadableFile(t testing.TB, path string) func(bool) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if err := os.Chmod(path, info.Mode().Perm()); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(restore)
	return func(readable bool) {
		t.Helper()
		mode := os.FileMode(0)
		if readable {
			mode = info.Mode().Perm()
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}
