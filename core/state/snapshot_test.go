package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM3A1SnapshotSelectionAndInputRefusal(t *testing.T) {
	for _, mode := range []string{"valid", "unsafe-id", "missing-scope", "bad-json", "null", "utf8", "tail", "oversized", "symlink-file", "symlink-agent", "bad-seen", "nonempty-seen", "too-many-seen", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			store, _ := New(source)
			sid := "session"
			if mode == "unsafe-id" {
				sid = "../../unsafe/session"
			}
			_, scope, err := store.Update(sid, "agent", EventUpdate{Source: "compact", PromptSubmitted: true})
			if err != nil {
				t.Fatal(err)
			}
			_, _, _ = store.Update("unrelated", "other", EventUpdate{})
			_, _, err = store.Claim(ClaimInput{Scope: scope, Rule: "pkg/rule", Action: "inject", Repeat: "session", Body: "body"})
			if err != nil {
				t.Fatal(err)
			}
			sessionDir := filepath.Join(source, component(sid))
			agentDir := filepath.Join(sessionDir, "agent")
			scopePath := filepath.Join(agentDir, "scope.json")
			seen := filepath.Join(agentDir, "seen")
			ctx := context.Background()
			switch mode {
			case "missing-scope":
				_ = os.Remove(scopePath)
			case "bad-json":
				_ = os.WriteFile(scopePath, []byte("{"), 0600)
			case "null":
				_ = os.WriteFile(scopePath, []byte("null"), 0600)
			case "utf8":
				_ = os.WriteFile(filepath.Join(sessionDir, "session.json"), []byte("{\"model\":\"\xff\"}"), 0600)
			case "tail":
				data, _ := os.ReadFile(scopePath)
				_ = os.WriteFile(scopePath, append(data, []byte("{}")...), 0600)
			case "oversized":
				_ = os.WriteFile(scopePath, []byte(strings.Repeat(" ", 65537)), 0600)
			case "symlink-file":
				target := filepath.Join(root, "foreign")
				_ = os.WriteFile(target, []byte("{}"), 0600)
				_ = os.Remove(scopePath)
				if err := os.Symlink(target, scopePath); err != nil {
					t.Fatal(err)
				}
			case "symlink-agent":
				target := filepath.Join(root, "foreign")
				_ = os.Mkdir(target, 0700)
				_ = os.RemoveAll(agentDir)
				if err := os.Symlink(target, agentDir); err != nil {
					t.Fatal(err)
				}
			case "bad-seen":
				_ = os.WriteFile(filepath.Join(seen, "unexpected"), nil, 0600)
			case "nonempty-seen":
				_ = os.WriteFile(filepath.Join(seen, "s.0.0.0000000000000000.0000000000000000"), []byte("x"), 0600)
			case "too-many-seen":
				for i := 0; i < 10001; i++ {
					if err := os.WriteFile(filepath.Join(seen, fmt.Sprintf("s.0.0.%016x.0000000000000000", i)), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			destination := filepath.Join(root, "destination")
			snapshot, err := CopySnapshot(ctx, source, destination, sid, "agent")
			valid := mode == "valid" || mode == "unsafe-id" || mode == "missing-scope"
			if valid {
				if err != nil || snapshot.Files == 0 {
					t.Fatalf("snapshot %+v %v", snapshot, err)
				}
				if snapshot.ScopeExists != (mode != "missing-scope") {
					t.Fatal("scope existence")
				}
				if _, err := os.Stat(filepath.Join(destination, "unrelated")); !os.IsNotExist(err) {
					t.Fatal("unrelated state copied")
				}
			} else if err == nil {
				t.Fatalf("accepted %s", mode)
			}
		})
	}
}
