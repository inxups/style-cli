package install

import (
	"os"
	"runtime"
	"testing"
)

func fixtureMode(want os.FileMode) os.FileMode {
	if runtime.GOOS == "windows" {
		if want&0200 == 0 {
			return 0444
		}
		return 0666
	}
	return want
}
func requirePOSIXFixture(t testing.TB, reason string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("D11: " + reason)
	}
}
