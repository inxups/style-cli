//go:build !windows

package testfs

import (
	"os"
	"testing"
)

func MakePublic(t testing.TB, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
