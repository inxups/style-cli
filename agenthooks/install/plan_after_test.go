package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"encoding/json"
	"github.com/speakeasy-api/agenthooks"
	"runtime"
	"strings"
)

func TestPlanFilesAfterComposesSharedConfigWithoutWrites(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: dir}
	manifest := testManifest()
	manifest.Owner = "style"
	initial := []byte("model = 'external'")
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, initial, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), manifest, target); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	remove, err := PlanUninstallFiles(manifest, target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Command = []string{"/tmp/new-hooks"}
	combined, err := PlanFilesAfter(manifest, target, remove)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(current, before) {
		t.Fatal("planning wrote configuration")
	}
	for _, file := range combined {
		if file.Path == path && !bytes.Equal(file.Before, before) {
			t.Fatal("original Before snapshot lost")
		}
	}
	hooksBefore, _ := os.ReadFile(filepath.Join(dir, "hooks.json"))
	intervening := append(append([]byte{}, before...), []byte("\n# intervening edit\n")...)
	if err := os.WriteFile(path, intervening, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := ApplyFiles(context.Background(), combined); err == nil {
		t.Fatal("stale combined plan applied")
	}
	hooksAfter, _ := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if !bytes.Equal(hooksBefore, hooksAfter) {
		t.Fatal("failed transaction changed hook configuration")
	}
	if err := os.WriteFile(path, before, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := ApplyFiles(context.Background(), combined); err != nil {
		t.Fatal(err)
	}
	hooks, _ := os.ReadFile(filepath.Join(dir, "hooks.json"))
	check := string(hooks)
	if runtime.GOOS == "windows" {
		var document map[string]any
		if json.Unmarshal(hooks, &document) != nil {
			t.Fatal("invalid configuration")
		}
		var decoded []string
		for _, v := range document["hooks"].(map[string]any) {
			for _, group := range v.([]any) {
				for _, h := range group.(map[string]any)["hooks"].([]any) {
					c := h.(map[string]any)["command"].(string)
					_, ok := windowsDecodedCommand(c)
					if !ok {
						t.Fatal("invalid encoded command")
					}
					decoded = append(decoded, fixtureCommandText(c))
				}
			}
		}
		check = strings.Join(decoded, "\n")
	}
	if !strings.Contains(check, "/tmp/new-hooks") || strings.Contains(check, "/usr/local/bin/myhooks") {
		t.Fatalf("hooks=%s", hooks)
	}
	remove, err = PlanUninstallFiles(manifest, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyFiles(context.Background(), remove); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, initial) {
		t.Fatalf("external bytes = %q want %q", after, initial)
	}
}

func TestCodexTrustKeysStayStableWhenNewHomeHasSymlinkedAncestor(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	alias := filepath.Join(root, "alias")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: filepath.Join(alias, "new-home")}
	before, err := Render(testManifest(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(real, "new-home"), 0o700); err != nil {
		t.Fatal(err)
	}
	after, err := Render(testManifest(), target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readRendered(t, before, "config.toml"), readRendered(t, after, "config.toml")) {
		t.Fatal("trust fingerprint changed after creating home")
	}
}
