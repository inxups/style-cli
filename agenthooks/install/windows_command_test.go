package install

import (
	"strings"
	"testing"
)

func TestM3A17WindowsEncodedOwnerBoundary(t *testing.T) {
	command := windowsHookCommand([]string{`C:\Program Files\中 %PATH%! $()\style.exe`, "agenthooks", "run", "--owner=style", "a'b"})
	if strings.ContainsAny(strings.TrimPrefix(command, windowsCommandPrefix), " %!&|<>()'\"$") {
		t.Fatal("outer shell can interpret encoded arguments")
	}
	script, ok := windowsDecodedCommand(command)
	if !ok || !strings.Contains(script, "'a''b'") || !commandHasOwner(command, "style") || commandHasOwner(command, "sty") {
		t.Fatal("encoded owner or quoting lost")
	}
	for _, bad := range []string{command + " --async", windowsCommandPrefix + "!!", windowsCommandPrefix + strings.Repeat("A", 40000), "other.exe " + strings.TrimPrefix(command, windowsCommandPrefix)} {
		if commandHasOwner(bad, "style") {
			t.Fatal("accepted damaged or foreign launcher")
		}
	}
}
