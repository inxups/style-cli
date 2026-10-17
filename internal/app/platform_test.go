package app

import (
	"encoding/json"
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

func fixtureNativeCount(posix int) int {
	if runtime.GOOS == "windows" {
		return 0
	}
	return posix
}

func fixtureRoutes(t *testing.T, routes []string) []string {
	t.Helper()
	var result []string
	for _, route := range routes {
		if runtime.GOOS == "windows" && (strings.HasSuffix(route, "-project") || strings.HasSuffix(route, "-plugin")) {
			t.Run("POSIX-route/"+route, func(t *testing.T) { requireFixtureRoute(t, route) })
		} else {
			result = append(result, route)
		}
	}
	return result
}
func fixtureChangedHook(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	groups := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] = "foreign-runtime --owner=style"
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
