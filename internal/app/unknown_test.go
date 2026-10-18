package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
)

func a12Manifest() string {
	return "spec: style/v0.3\nname: missing-data\nversion: 1.0.0\ndescription: missing data fixture\nreads: {model: true}\nrules:\n  - {id: r, target: event, event: tool.pre, when: {model: unknown}, text: 'Unknown rule'}\n"
}

func TestM3A12SixRouteUnknownLifecycleAndMarkerRemoval(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user", "claude-plugin", "codex-project", "claude-project"} {
		t.Run(route, func(t *testing.T) {
			requireFixtureRoute(t, route)
			harness := "codex"
			if strings.HasPrefix(route, "claude-") {
				harness = "claude-code"
			}
			var s *Service
			var home, source, config string
			if strings.HasSuffix(route, "-project") {
				var base, root string
				s, base, root, source = a6Setup(t)
				s = a6Scope(t, s, root, base)
				home = s.project.Home
			} else {
				fixture := route
				if route == "claude-plugin" {
					fixture = "claude-user"
				}
				s, home, config, source, _ = nativeInstallFixture(t, fixture)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a12Manifest())
			installed, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if route == "claude-plugin" {
				a8Registration(t, installed.Registration[0], config, "enabled")
			}
			lock, err := s.loadLock(home)
			if err != nil || lock.MinIndexFormat != 5 || lock.Format < 2 {
				t.Fatal(lock, err)
			}
			data := readSyncFile(t, filepath.Join(home, "build/index.json"))
			var index compile.Index
			if err = json.Unmarshal(data, &index); err != nil || index.Format != 5 {
				t.Fatal(index.Format, err)
			}
			if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			if r, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			check := a5Lock(t, home)
			a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
			if _, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
				t.Fatal(err)
			}
			check()
			if !bytes.Equal(data, readSyncFile(t, filepath.Join(home, "build/index.json"))) {
				t.Fatal("frozen changed unknown index")
			}
			d, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
			if err != nil || !d.Healthy {
				t.Fatal(d.Diagnostics, err)
			}
			found := false
			for _, v := range d.Diagnostics {
				found = found || v.Code == "runtime-index-5"
			}
			if !found {
				t.Fatal(d.Diagnostics)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.Replace(a12Manifest(), "when: {model: unknown}, ", "", 1))
			if _, err = s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			lock, err = s.loadLock(home)
			if err != nil || lock.MinIndexFormat != 0 || bytes.Contains(readSyncFile(t, filepath.Join(home, "style.lock.yaml")), []byte("min_index_format")) {
				t.Fatal(lock, err)
			}
			if route == "claude-plugin" {
				a8Registration(t, installed.Registration[0], config, "disabled")
			}
			if _, err = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestM3A12UnknownExplainCanonicalValuesAndSnapshot(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, field := range []string{"model", "provider", "thinking_effort", "harness.permission_mode"} {
			for _, mode := range []string{"missing", "empty", "known", "cached", "unmapped"} {
				if mode == "cached" && field != "model" && field != "thinking_effort" || mode == "unmapped" && field != "thinking_effort" {
					continue
				}
				t.Run(harness+"/"+field+"/"+mode, func(t *testing.T) {
					entry := a1Entry("unknown", "tool.pre", "Unknown rule")
					switch field {
					case "model":
						entry.When.Model = []string{"unknown"}
					case "provider":
						entry.When.Provider = []string{"unknown"}
					case "thinking_effort":
						entry.When.ThinkingEffort = []string{"unknown"}
					default:
						entry.When.Harness = map[string][]string{"permission_mode": {"unknown"}}
					}
					entry.Needs = []string{field}
					s, req := a1Fixture(t, harness, "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
					req.StateDir = t.TempDir()
					writeSyncFile(t, req.Index, string(bytes.Replace(readSyncFile(t, req.Index), []byte(`"format": 1`), []byte(`"format": 5`), 1)))
					payload := map[string]any{"session_id": "s", "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "true"}}
					facts := map[string]any{}
					value := ""
					if mode == "known" {
						value = "known-model"
						if field == "thinking_effort" {
							value = "high"
						}
					}
					if mode == "unmapped" {
						value = "future-tier"
					}
					if mode != "missing" && mode != "cached" {
						switch field {
						case "provider":
							facts["model_provider"] = value
						case "thinking_effort":
							key := "codex_config_effort"
							if harness == "claude-code" {
								key = "claude_env_effort"
							}
							facts[key] = value
						case "harness.permission_mode":
							payload["permission_mode"] = value
						default:
							payload[field] = value
						}
					}
					data, _ := json.Marshal(payload)
					writeSyncFile(t, req.Payload, string(data))
					a1Facts(t, &req, facts)
					if mode == "cached" {
						req.StateDir = t.TempDir()
						store, _ := state.New(req.StateDir)
						_, _, err := store.Update("s", "main", state.EventUpdate{Model: "known-model", ThinkingEffort: "high", EffortRaw: "high", EffortResolved: true})
						if err != nil {
							t.Fatal(err)
						}
					}
					before := snapshotStatsTree(t, filepath.Dir(req.Index))
					beforeState := snapshotStatsTree(t, req.StateDir)
					result, err := s.Explain(context.Background(), req)
					want := mode != "known" && mode != "cached"
					if err != nil || result.Wire == nil || strings.Contains(result.Wire.Stdout, "Unknown rule") != want {
						t.Fatal(result, err)
					}
					if !reflect.DeepEqual(before, snapshotStatsTree(t, filepath.Dir(req.Index))) || !reflect.DeepEqual(beforeState, snapshotStatsTree(t, req.StateDir)) {
						t.Fatal("explain mutated source")
					}
					if want {
						found := false
						for _, c := range result.Entries[0].Conditions {
							if c.Field == field {
								found = true
								if c.Available || c.Result != "pass" || c.Source != "unavailable" {
									t.Fatal(c)
								}
							}
						}
						if !found {
							t.Fatal(result.Entries)
						}
					}
				})
			}
		}
	}
}

func TestM3A12FrozenMarkerMismatchIsReadOnly(t *testing.T) {
	for _, replacement := range []string{"", "min_index_format: 6\n", "min_index_format: 1\n"} {
		t.Run(replacement, func(t *testing.T) {
			s, home, _, source, _ := nativeInstallFixture(t, "codex-user")
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a12Manifest())
			if _, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{"codex"}, Yes: true}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, "style.lock.yaml")
			writeSyncFile(t, path, strings.Replace(string(readSyncFile(t, path)), "min_index_format: 5\n", replacement, 1))
			before := snapshotStatsTree(t, home)
			if _, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err == nil {
				t.Fatal("frozen accepted missing/invalid minimum marker")
			}
			if !reflect.DeepEqual(before, snapshotStatsTree(t, home)) {
				t.Fatal("failed frozen mutated installation")
			}
		})
	}
	if _, err := decodeLock([]byte("format: 1\nmin_index_format: 5\npackages: []\n")); err == nil {
		t.Fatal("non-strict lock accepted minimum marker")
	}
}
