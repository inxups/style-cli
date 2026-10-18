package conformance_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/inxups/style-cli/core/state"
)

func TestNativeRulesLeaveRuntimeOutputsAndStatsUnchanged(t *testing.T) {
	baseline := read(t, filepath.Join(corpus, "STYLE.yaml"))
	manifest := append(append([]byte(nil), baseline...), []byte(`native_hooks:
  - id: native-output-boundary
    codex: {event: PreToolUse, matcher: 'Bash', command: 'printf STYLE_NATIVE_MUST_NOT_RUN'}
    claude-code: {event: PreToolUse, matcher: 'Bash', command: 'printf STYLE_NATIVE_MUST_NOT_RUN'}
`)...)
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			index := buildIndex(t, suite, manifest)
			baselineIndex := buildIndex(t, suite, baseline)
			// Inline rules authenticate their entire source file. Adding a native
			// declaration changes that digest, but no runtime behavior or body.
			for _, events := range baselineIndex.Harnesses {
				for point, entries := range events {
					for i := range entries {
						if entries[i].SourceSHA256 != fmt.Sprintf("sha256:%x", sha256.Sum256(baseline)) {
							t.Fatal("fixture does not use the inline manifest as its source")
						}
						entries[i].SourceSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
					}
					events[point] = entries
				}
			}
			if !reflect.DeepEqual(index, baselineIndex) {
				t.Fatal("native declarations changed runtime entries beyond their source digest")
			}
			paths, err := filepath.Glob(filepath.Join(corpus, string(suite.id), "*.json"))
			if err != nil || len(paths) == 0 {
				t.Fatalf("corpus=%v err=%v", paths, err)
			}
			for _, path := range paths {
				t.Run(filepath.Base(path), func(t *testing.T) {
					root := t.TempDir()
					store, err := state.New(root)
					if err != nil {
						t.Fatal(err)
					}
					result, _ := invoke(t, suite, index, read(t, path), store)
					goldenPath := path[:len(path)-len(".json")] + ".stdout.golden"
					if !bytes.Equal(result.Stdout, read(t, goldenPath)) {
						t.Fatalf("native changed output: got=%s expected=%s", result.Stdout, read(t, goldenPath))
					}
					checkWhitelist(t, suite.id, result.Stdout)
					baselineRoot := t.TempDir()
					baselineStore, err := state.New(baselineRoot)
					if err != nil {
						t.Fatal(err)
					}
					invoke(t, suite, baselineIndex, read(t, path), baselineStore)
					if got, want := runtimeStats(t, root), runtimeStats(t, baselineRoot); !reflect.DeepEqual(got, want) {
						t.Fatalf("native changed runtime stats: got=%+v expected=%+v", got, want)
					}
				})
			}
		})
	}
}

func runtimeStats(t *testing.T, root string) []state.StatsRecord {
	t.Helper()
	result := []state.StatsRecord{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "stats.jsonl" {
			for _, line := range bytes.Split(bytes.TrimSpace(read(t, path)), []byte("\n")) {
				var record state.StatsRecord
				if err := json.Unmarshal(line, &record); err != nil {
					return err
				}
				record.At = "" // Invocations occur at different wall-clock times.
				result = append(result, record)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}
