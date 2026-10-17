// This executable is an argv fixture named codex-fixture. It never invokes a
// model or a native harness; it only gives a configured hook a controlled parent.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

func main() {
	var input struct {
		Hook struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"hook"`
		CWD     string `json:"cwd"`
		Payload string `json:"payload"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(91)
	}
	args := input.Hook.Args
	for i := range args {
		args[i] = strings.ReplaceAll(args[i], "${CLAUDE_PROJECT_DIR}", input.CWD)
	}
	command := input.Hook.Command
	if len(args) == 0 {
		args = []string{"-c", command}
		command = "/bin/sh"
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = input.CWD
	cmd.Env = append(os.Environ(), "PWD="+input.CWD, "CLAUDE_PROJECT_DIR="+input.CWD)
	cmd.Stdin = strings.NewReader(input.Payload)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		os.Exit(92)
	}
}
