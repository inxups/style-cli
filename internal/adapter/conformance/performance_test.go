package conformance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/state"
)

// Run separately from competing test/build work. Measures exec start through
// process exit, including index load, decode, state, evaluation and encoding.
// Compilation and shell/provider startup are outside the measurement.
func TestHookProcessLatency(t *testing.T) {
	if os.Getenv("STYLE_PERF") != "1" {
		t.Skip("set STYLE_PERF=1 for the isolated A20 process benchmark")
	}
	binary := buildBinary(t)
	results := map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "samples_per_provider": 200, "threshold_ms": 50, "measurement": "exec start to process exit; warm filesystem/session; no shell; index and state only"}
	for _, suite := range suites() {
		home := t.TempDir()
		writeIndexOnlyHome(t, home, buildIndex(t, suite, read(t, filepath.Join(corpus, "STYLE.yaml"))))
		payload := read(t, filepath.Join(corpus, string(suite.id), "pre_tool_use.json"))
		var event struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		store, err := state.New(filepath.Join(home, "state"))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Update(event.SessionID, "", state.EventUpdate{EffortResolved: true}); err != nil {
			t.Fatal(err)
		}
		environment, args := processEnv(home), hookArgs(home, suite)
		want := read(t, filepath.Join(corpus, string(suite.id), "pre_tool_use.stdout.golden"))
		samples := make([]time.Duration, 0, 200)
		for sample := 0; sample < 210; sample++ {
			command := exec.Command(binary, args...)
			command.Env, command.Stdin = environment, bytes.NewReader(payload)
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			start := time.Now()
			err := command.Run()
			elapsed := time.Since(start)
			if err != nil || !bytes.Equal(stdout.Bytes(), want) {
				t.Fatalf("hook %s failed during benchmark: %v stdout=%s stderr=%s", suite.id, err, stdout.String(), stderr.String())
			}
			if sample >= 10 {
				samples = append(samples, elapsed)
			}
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		p95 := samples[(len(samples)*95+99)/100-1]
		median := samples[len(samples)/2]
		results[string(suite.id)] = map[string]any{"median_ms": float64(median) / float64(time.Millisecond), "p95_ms": float64(p95) / float64(time.Millisecond), "max_ms": float64(samples[len(samples)-1]) / float64(time.Millisecond)}
		t.Logf("%s: n=%d process median=%s p95=%s max=%s", suite.id, len(samples), median, p95, samples[len(samples)-1])
		if p95 >= 50*time.Millisecond {
			t.Errorf("%s process p95 %s must be below 50ms", suite.id, p95)
		}
	}
	if destination := os.Getenv("STYLE_PERF_OUTPUT"); destination != "" {
		raw, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
