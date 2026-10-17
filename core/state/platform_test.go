package state

import (
	"github.com/inxups/style-cli/internal/testfs"
	"os"
	"runtime"
	"testing"
)

// Windows exposes read-only attributes as mode bits. DACL privacy is separately
// checked by the Windows runtime integration tests; this helper asserts attributes.
func fixtureMode(want os.FileMode, dir bool) os.FileMode {
	if runtime.GOOS != "windows" {
		return want
	}
	if dir {
		return 0777
	}
	if want&0200 == 0 {
		return 0444
	}
	return 0666
}

func fixtureWritableDirectory(t *testing.T, path string) func(bool) {
	return testfs.WritableDirectory(t, path)
}
