//go:build windows

package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestM3A17WindowsChildHelper(t *testing.T) {
	if os.Getenv("STYLE_WINDOWS_PROCESS_HELPER") != "1" {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	index := 0
	for i, arg := range os.Args {
		if arg == "--" {
			index = i + 1
			break
		}
	}
	json.NewEncoder(os.Stdout).Encode(struct {
		Args  []string
		Input string
	}{os.Args[index:], string(data)})
	os.Exit(7)
}
func TestM3A17WindowsShellPreservesProcessContract(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "must-not-exist")
	if err := os.WriteFile(filepath.Join(root, "powershell.exe"), []byte("not a program"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"中文 🧪", "", "a'b", `trailing\`, `%PATH% ! & | < > ( ) ^`, "$(New-Item '" + marker + "')", "--owner=style"}
	command := windowsHookCommand(append([]string{os.Args[0], "-test.run=^TestM3A17WindowsChildHelper$", "--"}, args...))
	for _, shell := range []string{"cmd.exe", "powershell.exe"} {
		t.Run(shell, func(t *testing.T) {
			var cmd *exec.Cmd
			if shell == "cmd.exe" {
				cmd = exec.Command(shell, "/d", "/s", "/c", command)
			} else {
				cmd = exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", command)
			}
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "STYLE_WINDOWS_PROCESS_HELPER=1", "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
			input := "{\"值\":\"中😀\"}\r\n"
			cmd.Stdin = strings.NewReader(input)
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 7 {
				t.Fatalf("exit %v stderr %s", err, stderr.String())
			}
			var got struct {
				Args  []string
				Input string
			}
			if err = json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err, out.String(), stderr.String())
			}
			if !reflect.DeepEqual(got.Args, args) || got.Input != input {
				t.Fatalf("args/stdin changed: %#v %#v", got.Args, got.Input)
			}
			if _, err = os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("argument executed", err)
			}
		})
	}
}
