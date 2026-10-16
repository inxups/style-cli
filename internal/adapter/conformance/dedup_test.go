package conformance_test

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestCrossPackageDedupPreservesAllCodecGoldensAndRuntimeStats(t *testing.T) {
	manifest := read(t, filepath.Join(corpus, "STYLE.yaml"))
	for _, suite := range suites() {
		t.Run(string(suite.id), func(t *testing.T) {
			baseline := buildIndex(t, suite, manifest)
			var inputs []compile.PackageInput
			for i, name := range []string{"conformance", "duplicate"} {
				data := bytes.Replace(manifest, []byte("name: conformance"), []byte("name: "+name), 1)
				pkg, err := spec.LoadSnapshot("/conformance", map[string][]byte{"STYLE.yaml": data}, "")
				if err != nil {
					t.Fatal(err)
				}
				resolved, err := spec.Merge(pkg)
				if err != nil {
					t.Fatal(err)
				}
				planner, err := adapter.BuildPlanAdapter(suite.id, suite.facts)
				if err != nil {
					t.Fatal(err)
				}
				report, err := plan.Build(resolved, planner)
				if err != nil {
					t.Fatal(err)
				}
				inputs = append(inputs, compile.PackageInput{Package: resolved, Sequence: i + 1, Lock: "sha256:" + strings.Repeat("0", 64), Plans: []plan.Report{report}})
			}
			compiled, err := compile.Build(compile.Request{StyleVersion: "0.3.0", Packages: inputs})
			if err != nil {
				t.Fatal(err)
			}
			index := compiled.Index
			if len(index.Packages) != 2 || len(compiled.Deduplicated) == 0 || !reflect.DeepEqual(index.Harnesses, baseline.Harnesses) || !reflect.DeepEqual(index.Static, baseline.Static) {
				t.Fatal("duplicate package changed canonical runtime entries")
			}
			paths, err := filepath.Glob(filepath.Join(corpus, string(suite.id), "*.json"))
			if err != nil || len(paths) == 0 {
				t.Fatalf("payloads=%v %v", paths, err)
			}
			for _, path := range paths {
				t.Run(filepath.Base(path), func(t *testing.T) {
					root := t.TempDir()
					store, err := state.New(root)
					if err != nil {
						t.Fatal(err)
					}
					result, _ := invoke(t, suite, index, read(t, path), store)
					golden := path[:len(path)-len(".json")] + ".stdout.golden"
					if !bytes.Equal(result.Stdout, read(t, golden)) {
						t.Fatalf("dedup changed wire output: got=%s want=%s", result.Stdout, read(t, golden))
					}
					checkWhitelist(t, suite.id, result.Stdout)
					baselineRoot := t.TempDir()
					baselineStore, err := state.New(baselineRoot)
					if err != nil {
						t.Fatal(err)
					}
					invoke(t, suite, baseline, read(t, path), baselineStore)
					got, want := runtimeStats(t, root), runtimeStats(t, baselineRoot)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("alias produced telemetry: got=%+v want=%+v", got, want)
					}
					for _, record := range got {
						if strings.HasPrefix(record.Entry, "duplicate/") {
							t.Fatal("compiled alias generated a runtime log row")
						}
					}
				})
			}
		})
	}
}
