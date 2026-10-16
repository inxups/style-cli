package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHookCommandMakesRelativePathsAbsolute(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for command, want := range map[string]string{
		"./style":        filepath.Join(cwd, "style"),
		"bin/style":      filepath.Join(cwd, "bin", "style"),
		"../bin/style":   filepath.Join(filepath.Dir(cwd), "bin", "style"),
		"style":          "style",
		"/usr/bin/style": "/usr/bin/style",
	} {
		if got := HookCommand(command); got != want {
			t.Errorf("HookCommand(%q) = %q, want %q", command, got, want)
		}
	}
	saved := os.Args[0]
	defer func() { os.Args[0] = saved }()
	os.Args[0] = "./style"
	if got := HookCommand(""); got != filepath.Join(cwd, "style") {
		t.Fatalf("HookCommand(\"\") with os.Args[0]=./style = %q", got)
	}
}
