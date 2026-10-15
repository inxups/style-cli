package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExamplePlanMatchesProviderGoldens(t *testing.T) {
	root := repositoryRoot(t)
	service, err := DefaultService()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		provider string
		golden   string
	}{
		{provider: "codex", golden: "plan-codex.txt"},
		{provider: "claude-code", golden: "plan-claude-code.txt"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			var output, errOutput bytes.Buffer
			code := RunWithService(context.Background(), service, root, []string{
				"plan", "example/terse-reviewer", "--harness", test.provider,
			}, &output, &errOutput)
			if code != ExitOK {
				t.Fatalf("plan exit = %d, stderr=%s, stdout=%s", code, errOutput.String(), output.String())
			}
			golden, err := os.ReadFile(filepath.Join(root, "testdata", "golden", test.golden))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(output.Bytes(), golden) {
				t.Fatalf("plan output differs from %s", test.golden)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
