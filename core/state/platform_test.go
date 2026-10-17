package state

import (
	"os"
	"runtime"
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
