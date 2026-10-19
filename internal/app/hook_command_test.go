package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHookCommandRejectsUnsupportedInvocationWithoutStdout(t *testing.T) {
	for _, args := range [][]string{
		{"agenthooks"}, {"agenthooks", "serve", "--provider=codex"}, {"agenthooks", "run"},
		{"agenthooks", "run", "--provider=cursor"}, {"agenthooks", "run", "--provider=codex", "--provider=claude-code"},
		{"agenthooks", "run", "--provider=codex", "--fail-closed"}, {"agenthooks", "run", "--provider=claude-code", "--fail-closed=bad"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunWithInput(context.Background(), args, strings.NewReader(`{}`), &stdout, &stderr)
			if code != ExitInvalid || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("invalid hook code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}
