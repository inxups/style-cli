package adapter

import (
	"os"
	"path/filepath"
)

// HookCommand returns the program a harness hook runs. An empty command means
// the running program, os.Args[0]. A relative path such as ./style is made
// absolute, because the harness runs hooks from other working directories; a
// bare name such as style is left for the harness's PATH lookup.
func HookCommand(command string) string {
	if command == "" {
		command = os.Args[0]
	}
	if filepath.IsAbs(command) || filepath.Base(command) == command {
		return command
	}
	if absolute, err := filepath.Abs(command); err == nil {
		return absolute
	}
	return command
}
