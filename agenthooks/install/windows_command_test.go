package install

import (
	"strings"
	"testing"
)

func TestM3A17WindowsEncodedOwnerBoundary(t *testing.T) {
	command := windowsHookCommand([]string{`C:\Program Files\中 %PATH%! $()\style.exe`, "agenthooks", "run", "--owner=style", "a'b"})
	if strings.ContainsAny(strings.TrimPrefix(command, windowsCommandPrefix()), " %!&|<>()'\"$") {
		t.Fatal("outer shell can interpret encoded arguments")
	}
	script, ok := windowsDecodedCommand(command)
	if !ok || !strings.Contains(script, "a'b") || !commandHasOwner(command, "style") || commandHasOwner(command, "sty") {
		t.Fatal("encoded owner or quoting lost")
	}
	for _, bad := range []string{command + " --async", windowsCommandPrefix() + "!!", windowsCommandPrefix() + strings.Repeat("A", 40000), "other.exe " + strings.TrimPrefix(command, windowsCommandPrefix())} {
		if commandHasOwner(bad, "style") {
			t.Fatal("accepted damaged or foreign launcher")
		}
	}
}

func TestM3A17WindowsOwnerIsAnArgument(t *testing.T) {
	for _, args := range [][]string{{"--owner=style"}, {"agenthooks", "--owner=style", ""}, {"agenthooks", "value", "--owner=style"}} {
		if !commandHasOwner(windowsHookCommand(append([]string{"style.exe"}, args...)), "style") {
			t.Fatal("lost owner", args)
		}
	}
	for _, args := range [][]string{{" --owner=style "}, {"--owner=other"}, {`a "--owner=style" b`}, {}} {
		if commandHasOwner(windowsHookCommand(append([]string{`C:\ with --owner=style \style.exe`}, args...)), "style") {
			t.Fatal("file name/value mistaken for owner", args)
		}
	}
}
