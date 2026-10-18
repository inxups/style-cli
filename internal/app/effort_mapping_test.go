package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/speakeasy-api/agenthooks"
)

const a13Manifest = "spec: style/v0.3\nname: effort-policy\nversion: 1.0.0\ndescription: effort policy fixture\nrules:\n  - {id: r, target: event, event: tool.pre, repeat: every, text: 'Effort={{thinking_effort | default: \"missing\"}}/raw={{thinking_effort_raw | default: \"missing\"}}'}\n"

func a13Config(harness, tier string) string {
	return "format: 1\nharnesses:\n  " + harness + ": {future: " + tier + ", high: low}\n"
}

func TestM3A13StrictConfigurationAndBoundedCapture(t *testing.T) {
	s := testService(t)
	cases := map[string]string{"unknown-field": "format: 1\nprivate: value\n", "format": "format: 2\nharnesses: {}\n", "duplicate": "format: 1\nformat: 1\n", "multiple": "format: 1\n---\nformat: 1\n", "unknown-harness": "format: 1\nharnesses: {other: {future: high}}\n", "conflict": "format: 1\nharnesses: {codex: {FUTURE: high, future: low}}\n", "bad-tier": a13Config("codex", "unknown"), "bad-key": "format: 1\nharnesses: {codex: {../private: high}}\n", "wrong-type": "format: 1\nharnesses: {codex: {future: [high]}}\n", "oversize": strings.Repeat(" ", 65537), "empty": ""}
	many := "format: 1\nharnesses:\n  codex:\n"
	for i := range 129 {
		many += fmt.Sprintf("    value%d: high\n", i)
	}
	cases["too-many"] = many
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeSyncFile(t, filepath.Join(home, "effort.yaml"), data)
			before := snapshotStatsTree(t, home)
			if _, err := s.captureEffortMapping(home, nil); err == nil {
				t.Fatal("accepted invalid configuration")
			}
			if !reflect.DeepEqual(before, snapshotStatsTree(t, home)) {
				t.Fatal("configuration read wrote files")
			}
		})
	}
	for _, mode := range []string{"missing", "normalized", "symlink", "directory"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "effort.yaml")
			switch mode {
			case "normalized":
				writeSyncFile(t, path, "format: 1\nharnesses: {codex: {' FUTURE ': ' HIGH '}}\n")
			case "symlink":
				target := filepath.Join(t.TempDir(), "foreign")
				writeSyncFile(t, target, a13Config("codex", "high"))
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			mapping, err := s.captureEffortMapping(home, nil)
			if mode == "symlink" || mode == "directory" {
				if err == nil {
					t.Fatal("nonregular config accepted")
				}
				return
			}
			if err != nil || mode == "missing" && len(mapping) != 0 || mode == "normalized" && mapping["codex"]["future"] != "high" {
				t.Fatal(mapping, err)
			}
		})
	}
}

func a13Route(t *testing.T, route string) (*Service, string, string, string, string) {
	t.Helper()
	harness := "codex"
	if strings.HasPrefix(route, "claude-") {
		harness = "claude-code"
	}
	if strings.HasSuffix(route, "-project") {
		s, base, root, source := a6Setup(t)
		s = a6Scope(t, s, root, base)
		return s, s.project.Home, source, "", harness
	}
	fixture := route
	if route == "claude-plugin" {
		fixture = "claude-user"
	}
	s, home, config, source, _ := nativeInstallFixture(t, fixture)
	return s, home, source, config, harness
}
func TestM3A13SixRouteMappingLifecycleFrozenAndRemoval(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user", "claude-plugin", "codex-project", "claude-project"} {
		t.Run(route, func(t *testing.T) {
			requireFixtureRoute(t, route)
			s, home, source, config, harness := a13Route(t, route)
			configPath := filepath.Join(home, "effort.yaml")
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a13Manifest)
			writeSyncFile(t, configPath, a13Config(harness, "high"))
			r, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if route == "claude-plugin" {
				a8Registration(t, r.Registration[0], config, "enabled")
			}
			lock, err := s.loadLock(home)
			if err != nil || lock.MinIndexFormat != 6 || lock.EffortMapping[harness]["future"] != "high" || lock.Format < 2 {
				t.Fatal(lock, err)
			}
			data := readSyncFile(t, filepath.Join(home, "build/index.json"))
			var index compile.Index
			if err = json.Unmarshal(data, &index); err != nil || index.Format != 6 {
				t.Fatal(index, err)
			}
			if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			if r, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			check := a5Lock(t, home)
			writeSyncFile(t, configPath, "INVALID LIVE CONFIGURATION")
			a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
			if _, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
				t.Fatal("frozen consulted live config", err)
			}
			check()
			if !bytes.Equal(data, readSyncFile(t, filepath.Join(home, "build/index.json"))) {
				t.Fatal("frozen changed approved mapping")
			}
			writeSyncFile(t, configPath, a13Config(harness, "medium"))
			doctor, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
			if err != nil || !doctor.Healthy {
				t.Fatal(doctor.Diagnostics, err)
			}
			found := false
			for _, d := range doctor.Diagnostics {
				found = found || d.Code == "effort-mapping-pending"
			}
			if !found {
				t.Fatal(doctor.Diagnostics)
			}
			if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || !r.Changed {
				t.Fatal(r, err)
			}
			lock, err = s.loadLock(home)
			if err != nil || lock.EffortMapping[harness]["future"] != "medium" {
				t.Fatal(lock, err)
			}
			writeSyncFile(t, configPath, a13Config(harness, "low"))
			if _, err = s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			lock, err = s.loadLock(home)
			if err != nil || lock.EffortMapping[harness]["future"] != "low" {
				t.Fatal(lock, err)
			}
			if err = os.Remove(configPath); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Sync(context.Background(), SyncRequest{Home: home}); err != nil {
				t.Fatal(err)
			}
			lock, err = s.loadLock(home)
			if err != nil || lock.MinIndexFormat != 0 || len(lock.EffortMapping) != 0 {
				t.Fatal(lock, err)
			}
			if route == "claude-plugin" {
				a8Registration(t, r.Registration[0], config, "disabled")
			}
			if _, err = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestM3A13MappingChangeDuringCommitIsRejected(t *testing.T) {
	s, home, source, _, harness := a13Route(t, "codex-user")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a13Manifest)
	path := filepath.Join(home, "effort.yaml")
	writeSyncFile(t, path, a13Config(harness, "high"))
	if _, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, path, a13Config(harness, "medium"))
	prepared, err := s.prepareSync(context.Background(), TrustRequest{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, path, a13Config(harness, "low"))
	before := a3Installed(t, home, home)
	if _, err = applySync(context.Background(), prepared); err == nil {
		t.Fatal("committed stale mapping capture")
	}
	if !reflect.DeepEqual(before, a3Installed(t, home, home)) {
		t.Fatal("stale commit changed files")
	}
	prepared, err = s.prepareSync(context.Background(), TrustRequest{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = applySync(ctx, prepared); err == nil {
		t.Fatal("cancelled mapping commit succeeded")
	}
	if !reflect.DeepEqual(before, a3Installed(t, home, home)) {
		t.Fatal("cancelled commit changed files")
	}
}

func TestM3A13CompiledMappingExplainAndNativeCodecParity(t *testing.T) {
	for _, h := range []string{"codex", "claude-code"} {
		for _, raw := range []string{"future", "high", "mystery", ""} {
			t.Run(h+"/"+raw, func(t *testing.T) {
				entry := a1Entry("effort", "tool.pre", "unused")
				entry.Repeat = "every"
				missing := "missing"
				entry.Body = compile.Body{Parts: []compile.Fragment{{Text: "Effort="}, {Variable: &compile.Variable{Name: "thinking_effort", Default: &missing}}, {Text: "/raw="}, {Variable: &compile.Variable{Name: "thinking_effort_raw", Default: &missing}}, {Text: "/approx="}, {Variable: &compile.Variable{Name: "thinking_effort_approx", Default: &missing}}}}
				entry.Needs = []string{"thinking_effort", "thinking_effort_raw", "thinking_effort_approx"}
				s, req := a1Fixture(t, h, "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
				var index compile.Index
				if err := json.Unmarshal(readSyncFile(t, req.Index), &index); err != nil {
					t.Fatal(err)
				}
				index.Format = 6
				index.EffortMapping = compile.EffortMapping{h: {"future": "high", "high": "low"}}
				data, _ := compile.Encode(index)
				writeSyncFile(t, req.Index, string(data))
				key := "codex_config_effort"
				if h == "claude-code" {
					key = "claude_env_effort"
				}
				a1Facts(t, &req, map[string]any{key: raw})
				req.StateDir = t.TempDir()
				before := snapshotStatsTree(t, filepath.Dir(req.Index))
				beforeState := snapshotStatsTree(t, req.StateDir)
				result, err := s.Explain(context.Background(), req)
				if err != nil || result.Wire == nil || result.Inputs.EffortMappingSHA256 != index.EffortMapping.SHA256(h) {
					t.Fatal(result, err)
				}
				if !reflect.DeepEqual(before, snapshotStatsTree(t, filepath.Dir(req.Index))) || !reflect.DeepEqual(beforeState, snapshotStatsTree(t, req.StateDir)) {
					t.Fatal("explain mutated inputs")
				}
				tier, approx, source := raw, "true", "user"
				switch raw {
				case "high":
					tier = "low"
				case "future":
					tier = "high"
				case "mystery", "":
					tier, approx, source = "missing", "missing", "unmapped"
				}
				rawText := raw
				if rawText == "" {
					rawText = "missing"
				}
				if !strings.Contains(result.Wire.Stdout, "Effort="+tier+"/raw="+rawText+"/approx="+approx) {
					t.Fatal(result.Wire)
				}
				for _, f := range result.Facts {
					if f.Field == "thinking_effort" && f.Available && f.Source != "mapping:"+source {
						t.Fatal(f)
					}
				}
				store, _ := state.New(t.TempDir())
				core, err := engine.New(index, store)
				if err != nil {
					t.Fatal(err)
				}
				var enricher adapter.RuntimeEnricher
				if h == "codex" {
					e := codex.NewEnricher()
					e.Offline = true
					e.OfflineConfigEffort = &raw
					enricher = e
				} else {
					e := claude.NewEnricher()
					e.Environment = func() string { return raw }
					enricher = e
				}
				runtime, err := styleruntime.New(core, store, enricher, agenthooks.Provider(h), false)
				if err != nil {
					t.Fatal(err)
				}
				runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
				runtime.CodecStateDir = t.TempDir()
				var out, stderr bytes.Buffer
				exit := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + h}, bytes.NewReader(readSyncFile(t, req.Payload)), &out, &stderr, agenthooks.WithoutMCPResolution())
				if exit != result.Wire.ExitCode || out.String() != result.Wire.Stdout || stderr.String() != result.Wire.Stderr {
					t.Fatal(exit, out.String(), stderr.String(), result.Wire)
				}
				session, _, err := store.Read("s", "main")
				if err != nil || session.EffortMappingSource != source || session.EffortMappingSHA256 != index.EffortMapping.SHA256(h) {
					t.Fatal(session, err)
				}
			})
		}
	}
}

func TestM3A13CachedMappingChangeRemovalAndMissingRaw(t *testing.T) {
	for _, h := range []string{"codex", "claude-code"} {
		for _, mode := range []string{"change", "remove", "remove-builtin", "missing-raw"} {
			t.Run(h+"/"+mode, func(t *testing.T) {
				entry := a1Entry("cache", "tool.pre", "unused")
				missing := "missing"
				entry.Body = compile.Body{Parts: []compile.Fragment{{Text: "Tier="}, {Variable: &compile.Variable{Name: "thinking_effort", Default: &missing}}}}
				s, req := a1Fixture(t, h, "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
				var index compile.Index
				if err := json.Unmarshal(readSyncFile(t, req.Index), &index); err != nil {
					t.Fatal(err)
				}
				index.EffortMapping = compile.EffortMapping{h: {"future": "low"}}
				index.Format = 6
				want := "low"
				raw := "future"
				if strings.HasPrefix(mode, "remove") {
					index.EffortMapping = nil
					index.Format = 1
					want = "missing"
				}
				if mode == "remove-builtin" {
					raw, want = "high", "high"
				}
				if mode == "missing-raw" {
					raw, want = "", "missing"
				}
				data, _ := compile.Encode(index)
				writeSyncFile(t, req.Index, string(data))
				req.StateDir = t.TempDir()
				store, _ := state.New(req.StateDir)
				approx := true
				old := compile.EffortMapping{h: {"future": "high"}}.SHA256(h)
				if _, _, err := store.Update("s", "main", state.EventUpdate{EffortMappingResolved: true, EffortMappingSHA256: old, EffortMappingSource: "user", EffortResolved: true, EffortRaw: raw, ThinkingEffort: "high", EffortApprox: &approx}); err != nil {
					t.Fatal(err)
				}
				result, err := s.Explain(context.Background(), req)
				if err != nil || result.Wire == nil || !strings.Contains(result.Wire.Stdout, "Tier="+want) {
					t.Fatal(result, err)
				}
			})
		}
	}
}

func TestM3A13PlanAndDoctorMappingProvenance(t *testing.T) {
	s, home, source, config, harness := a13Route(t, "codex-user")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a13Manifest)
	writeSyncFile(t, filepath.Join(home, "effort.yaml"), a13Config(harness, "high"))
	planned, err := s.Plan(context.Background(), PlanRequest{Source: source, Home: home, Harnesses: []string{harness}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range planned.Diagnostics {
		found = found || d.Code == "effort-mapping" && strings.Contains(d.Message, "sha256:")
	}
	if !found {
		t.Fatal(planned.Diagnostics)
	}
	if _, err = s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(config, "config.toml"), "model_reasoning_effort='high'\n")
	doctor, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, effort := range doctor.Effort {
		if effort.Provider == harness {
			found = true
			if effort.Tier != "low" || effort.MappingSource != "user" || effort.MappingSHA256 == "" || effort.Approximate == nil || !*effort.Approximate {
				t.Fatal(effort)
			}
		}
	}
	if !found {
		t.Fatal(doctor.Effort)
	}
}
