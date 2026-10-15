package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/state"
)

func TestDoctorDetectsStaticInstructionDrift(t *testing.T) {
	root := t.TempDir()
	home, configDir := filepath.Join(root, "style"), filepath.Join(root, "codex")
	service := testCodexInstallService(t, configDir, filepath.Join(root, "plugin"))
	source := writeInstallPackage(t, filepath.Join(root, "source"), "spec: style/v0.3\nname: static\nversion: 0.1.0\ndescription: test\nrules:\n  - id: base\n    target: system\n    text: Keep responses concise.\n")
	if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Yes: true, Harnesses: []string{"codex"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(data, []byte("Keep responses concise."), []byte("Changed static instruction."), 1)
	if bytes.Equal(data, changed) {
		t.Fatalf("test requires a static instruction: %s", data)
	}
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := service.Doctor(context.Background(), DoctorRequest{Home: home, Shell: "/bin/sh"})
	if err != nil || result.Healthy || !doctorHas(result, "route-drift") {
		t.Fatalf("static drift = %+v, %v", result, err)
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(current, changed) {
		t.Fatal("doctor modified the inspected route")
	}
}

func doctorHas(result DoctorResult, code string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestDoctorChecksIndexLockSettingsAndRouteIntegrity(t *testing.T) {
	for _, scenario := range []string{"healthy", "invalid-json", "valid-tamper", "stale-lock", "changed-source", "missing-route"} {
		t.Run(scenario, func(t *testing.T) {
			service, home := inspectionFixture(t)
			indexPath := filepath.Join(home, "build", "index.json")
			switch scenario {
			case "invalid-json":
				if err := os.WriteFile(indexPath, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "valid-tamper":
				data, err := os.ReadFile(indexPath)
				if err != nil {
					t.Fatal(err)
				}
				var index map[string]any
				if err := json.Unmarshal(data, &index); err != nil {
					t.Fatal(err)
				}
				index["style_version"] = "9.9.9"
				data, _ = json.Marshal(index)
				if err := os.WriteFile(indexPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
			case "stale-lock":
				lock, err := loadLock(home)
				if err != nil {
					t.Fatal(err)
				}
				lock.Packages[0].Seq++
				if err := writeLock(home, lock); err != nil {
					t.Fatal(err)
				}
			case "changed-source":
				if err := os.WriteFile(filepath.Join(home, "packages", "terse-reviewer", "extra.md"), []byte("edited"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing-route":
				if err := os.Remove(filepath.Join(home, "adapters", "codex", "plugin", "hooks", "hooks.json")); err != nil {
					t.Fatal(err)
				}
			}
			result, err := service.Doctor(context.Background(), DoctorRequest{Home: home, Shell: "/bin/sh"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Healthy != (scenario == "healthy") {
				t.Fatalf("healthy=%v diagnostics=%+v", result.Healthy, result.Diagnostics)
			}
			if scenario == "healthy" && !doctorHas(result, "route-trust-unverified") {
				t.Fatal("plugin trust must remain unverified")
			}
			if scenario != "healthy" && scenario != "missing-route" && !doctorHas(result, "index-invalid") {
				t.Fatalf("missing index finding: %+v", result.Diagnostics)
			}
			if scenario == "changed-source" && !doctorHas(result, "package-changed") || scenario == "missing-route" && !doctorHas(result, "route-missing") {
				t.Fatalf("missing finding: %+v", result.Diagnostics)
			}
		})
	}
}

func TestDoctorReportsEffortPoliciesAndNoisyLoginShell(t *testing.T) {
	service, home := inspectionFixture(t)
	lock, err := loadLock(home)
	if err != nil {
		t.Fatal(err)
	}
	locked := lock.Harnesses["codex"]
	locked.FailClosed = true
	lock.Harnesses["codex"] = locked
	lock.Packages[0].Replace = true
	if err := writeLock(home, lock); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(locked.ConfigDir, "config.toml")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, append([]byte("model_reasoning_effort = 'high'\n"), data...), 0o600); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(t.TempDir(), "noisy-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf 'private startup output'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := service.Doctor(context.Background(), DoctorRequest{Home: home, Shell: shell})
	if err != nil || result.Healthy || !doctorHas(result, "codex-login-shell") || !doctorHas(result, "replace-active") || !doctorHas(result, "fail-closed") {
		t.Fatalf("Doctor = %+v, %v", result, err)
	}
	if len(result.Effort) != 1 || result.Effort[0].Source != "user config" || result.Effort[0].Tier != "high" || result.Effort[0].Raw != "high" {
		t.Fatalf("effort = %+v", result.Effort)
	}
	data, _ = json.Marshal(result)
	if strings.Contains(string(data), "private startup output") {
		t.Fatal("doctor leaked shell stdout")
	}
}

func TestDoctorStateExpiryResetAndSymlinkBoundary(t *testing.T) {
	service, err := New()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	root := filepath.Join(home, "state")
	old := filepath.Join(root, "old")
	recent := filepath.Join(root, "recent")
	for _, dir := range []string{old, recent} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ago := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, ago, ago); err != nil {
		t.Fatal(err)
	}
	result, err := service.Doctor(context.Background(), DoctorRequest{Home: home})
	if err != nil || !result.Healthy || result.ExpiredSessions != 1 {
		t.Fatalf("expiry = %+v, %v", result, err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal(err)
	}
	result, err = service.Doctor(context.Background(), DoctorRequest{Home: home, ResetState: true})
	if err != nil || !result.StateReset {
		t.Fatalf("reset = %+v, %v", result, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("state survives reset: %v", err)
	}
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	result, err = service.Doctor(context.Background(), DoctorRequest{Home: home, ResetState: true})
	if err != nil || result.Healthy || !doctorHas(result, "state-invalid") {
		t.Fatalf("symlink = %+v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatalf("external state changed: %v", err)
	}
}

func TestDoctorCostsUseHistoricalMetadataAndSevenDayWindow(t *testing.T) {
	root := t.TempDir()
	store, _ := state.New(root)
	now := time.Now().UTC()
	add := func(rule, result string, tokens int, at time.Time, legacy bool) {
		t.Helper()
		record := state.StatsRecord{At: at.Format(time.RFC3339Nano), Provider: "codex", Event: "PreToolUse", Point: "tool.pre", Entry: "pkg/" + rule + "@tool.pre", Action: "inject", Result: result}
		if !legacy {
			record.Tokens, record.BodySHA256 = &tokens, "sha256:"+strings.Repeat("a", 64)
		}
		data, _ := json.Marshal(record)
		if err := store.AppendStats("session", "main", data); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		add("repeated", "output", 10, now, false)
	}
	add("big", "output", 50, now, false)
	add("small", "output", 2, now, false)
	add("fourth", "output", 1, now, false)
	add("old", "output", 999, now.Add(-8*24*time.Hour), false)
	add("future", "output", 999, now.Add(time.Hour), false)
	add("dedup", "dedup", 999, now, false)
	add("legacy", "output", 999, now, true)
	costs, legacy, err := readRuleCosts(context.Background(), root, now)
	if err != nil || len(costs) != 3 || legacy != 1 || costs[0].Rule != "pkg/big" || costs[1].EstimatedTokens != 30 || !costs[1].RepeatedBody || costs[1].Emits != 3 || costs[2].Rule != "pkg/small" {
		t.Fatalf("costs = %+v, legacy=%d err=%v", costs, legacy, err)
	}
}
