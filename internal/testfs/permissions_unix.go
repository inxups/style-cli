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
