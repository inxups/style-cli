//go:build !windows

package install

import (
	"os"
	"testing"
)

func exposeFixtureFile(t testing.TB, path string) {
	t.Helper()
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
}
