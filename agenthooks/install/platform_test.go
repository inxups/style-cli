package install

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
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

func fixtureCommandText(command string) string {
	if runtime.GOOS != "windows" {
		return command
	}
	_, encoded, ok := strings.Cut(command, windowsCommandOptions)
	if !ok {
		return command
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data)%2 != 0 {
		return "invalid command"
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	return string(utf16.Decode(units))
}
