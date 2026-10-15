package install

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/speakeasy-api/agenthooks"
)

func restoreFixture(t *testing.T, original []byte) (Manifest, Target, string, string) {
	t.Helper()
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser, Dir: dir, HarnessVersion: "2.1.315"}
	manifest := testManifest()
	manifest.Owner = "style"
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), manifest, target); err != nil {
		t.Fatal(err)
	}
	return manifest, target, path, filepath.Join(dir, restorePath("settings.json", "style"))
}

func TestJSONRestorationKeepsOriginalBytesAcrossUpdates(t *testing.T) {
	for _, original := range []string{`{}`, "{\"hooks\":{}}\r\n", "{ \"env\" : {\"KEEP\":\"\\u4fdd\\u7559\"} }\n\n"} {
		t.Run(original, func(t *testing.T) {
			manifest, target, path, recordPath := restoreFixture(t, []byte(original))
			if info, err := os.Stat(recordPath); err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("record permissions: %v %v", info, err)
			}
			if err := os.Chmod(recordPath, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Install(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			if info, err := os.Stat(recordPath); err != nil || info.Mode().Perm() != 0o600 {
				t.Fatal("record permissions were not repaired")
			}
			manifest.Command = []string{"/new/style", "with spaces"}
			if err := Install(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			if err := Uninstall(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != original {
				t.Fatalf("restored %q want %q: %v", got, original, err)
			}
			if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
				t.Fatalf("restoration record remains: %v", err)
			}
		})
	}
}

func TestJSONRestorationNeverOverwritesInterveningEdits(t *testing.T) {
	for _, reinstall := range []bool{false, true} {
		t.Run(map[bool]string{false: "uninstall", true: "reinstall then uninstall"}[reinstall], func(t *testing.T) {
			manifest, target, path, _ := restoreFixture(t, []byte(`{"env":{"KEEP":"old"}}`))
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			raw = bytes.ReplaceAll(raw, []byte(`"old"`), []byte(`"user edit"`))
			if err := os.WriteFile(path, raw, 0o640); err != nil {
				t.Fatal(err)
			}
			if reinstall {
				if err := Install(context.Background(), manifest, target); err != nil {
					t.Fatal(err)
				}
			}
			if err := Uninstall(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Contains(got, []byte("user edit")) || bytes.Contains(got, []byte("--owner=style")) {
				t.Fatalf("external edit lost: %s %v", got, err)
			}
		})
	}
}

func TestJSONRestorationRecordParticipatesInStaleCheck(t *testing.T) {
	manifest, target, path, recordPath := restoreFixture(t, []byte(`{"keep":true}`))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := PlanUninstallFiles(manifest, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, []byte("changed after planning"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyFiles(context.Background(), changes); err == nil {
		t.Fatal("stale record accepted")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, before) {
		t.Fatal("stale plan partially restored config")
	}
	if _, err := PlanFiles(manifest, target); err == nil {
		t.Fatal("malformed record accepted")
	}
	if _, err := PlanUninstallFiles(manifest, target); err == nil {
		t.Fatal("malformed record accepted during removal")
	}
}

func TestJSONRestorationDoesNotResurrectDeletedConfig(t *testing.T) {
	for _, reinstall := range []bool{false, true} {
		t.Run(map[bool]string{false: "uninstall", true: "reinstall"}[reinstall], func(t *testing.T) {
			manifest, target, path, recordPath := restoreFixture(t, []byte(`{"old":"must not return"}`))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if reinstall {
				if err := Install(context.Background(), manifest, target); err != nil {
					t.Fatal(err)
				}
			}
			if err := Uninstall(context.Background(), manifest, target); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{path, recordPath} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("deleted file resurrected: %s %v", path, err)
				}
			}
		})
	}
}

func TestJSONRestorationRecordsAreOwnerIsolated(t *testing.T) {
	manifest, target, path, _ := restoreFixture(t, []byte(`{"keep":true}`))
	other := manifest
	other.Owner = "other"
	other.Command = []string{"/other"}
	if err := Install(context.Background(), other, target); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(context.Background(), manifest, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(got), "--owner=other") || strings.Contains(string(got), "--owner=style") {
		t.Fatalf("other owner lost: %s %v", got, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil || parsed["keep"] != true {
		t.Fatalf("foreign root lost: %s %v", got, err)
	}
}
