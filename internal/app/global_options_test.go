package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

// --style-home replaces STYLE_HOME for user-scope commands, stats included.
func TestStyleHomeReplacesEnvironmentHome(t *testing.T) {
	work := t.TempDir()
	t.Setenv("STYLE_HOME", filepath.Join(work, "env-home"))
	service := testService(t)
	for _, command := range []string{"list", "doctor", "stats"} {
		var out, errOut bytes.Buffer
		RunWithService(context.Background(), service, work, []string{"--style-home", "flag-home", command, "--json"}, &out, &errOut)
		var envelope struct {
			Scope struct{ Kind, Home string }
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("%s: %v\n%s%s", command, err, &out, &errOut)
		}
		if envelope.Scope.Kind != "user" || envelope.Scope.Home != filepath.Join(work, "flag-home") {
			t.Errorf("%s: scope = %+v", command, envelope.Scope)
		}
	}
}
