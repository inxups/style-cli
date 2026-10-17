package app

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func requireFixtureRoute(t testing.TB, route string) {
	t.Helper()
	if runtime.GOOS == "windows" && (strings.HasSuffix(route, "-project") || strings.HasSuffix(route, "-plugin")) {
		t.Skip("D11: Windows supports user routes; POSIX project/plugin launcher fixture")
	}
}
func requirePOSIXFixture(t testing.TB, reason string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("D11: " + reason)
	}
}

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
