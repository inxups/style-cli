// windows-helper is a version-only harness and lossless process-contract fixture.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if strings.Contains(strings.ToLower(filepath.Base(os.Args[0])), "claude") {
			fmt.Println("2.1.301 (Claude Code)")
		} else {
			fmt.Println("codex-cli 0.159.3")
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "echo-contract" {
		code, _ := strconv.Atoi(os.Args[2])
		data, _ := io.ReadAll(os.Stdin)
		json.NewEncoder(os.Stdout).Encode(struct {
			Args  []string `json:"args"`
			Stdin string   `json:"stdin"`
		}{os.Args[3:], string(data)})
		os.Exit(code)
	}
	fmt.Fprintln(os.Stderr, "fixture only supports --version or echo-contract")
	os.Exit(1)
}
