package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
)

func a6Manifest(name string, priority float64) string {
	return fmt.Sprintf(`spec: style/v0.3
name: %s
version: 0.1.0
description: Cross-package dedup fixture
defaults: {priority: %g}
rules:
  - {id: static, target: system, text: Shared static instructions.}
  - {id: dynamic, target: system, text: 'Model {{model | default: "unknown"}}.'}
  - {id: prompt, target: event, event: prompt.submitted, text: Shared prompt instructions.}
  - id: guard
    target: tool
    tool: shell
    at: before
    action: block
    text: Shared denial.
    when:
      conditions: [{field: tool.input.command, op: contains, pattern: DANGER}]
  - {id: warning, target: event, event: tool.post, action: warn, text: 'Warning. More detail.'}
  - {id: child, target: event, event: subagent.start, text: Shared child instructions.}
`, name, priority)
}

func a6Index(t *testing.T, home string) compile.Index {
	t.Helper()
	var index compile.Index
	if err := json.Unmarshal(readSyncFile(t, filepath.Join(home, "build", "index.json")), &index); err != nil {
		t.Fatal(err)
	}
	return index
}

func a6AssertCanonical(t *testing.T, home, provider, canonical string, packageCount int) {
	t.Helper()
	index := a6Index(t, home)
	if len(index.Packages) != packageCount || len(index.Static[provider]) != 1 || index.Static[provider][0].Rule != canonical+"/static" {
		t.Fatalf("incorrect static canonical/packages: %+v", index)
	}
	count := 0
	for _, entries := range index.Harnesses[provider] {
		for _, entry := range entries {
			count++
			if !strings.HasPrefix(entry.Rule, canonical+"/") {
				t.Fatalf("alias in runtime index: %s", entry.Rule)
			}
		}
	}
	wantCount := 6
	if provider == "claude-code" {
		wantCount = 5
	} // SubagentStart lacks model context on this harness.
	if count != wantCount {
		t.Fatalf("runtime placement count=%d want=%d", count, wantCount)
	}
}

func a6AssertDoctor(t *testing.T, service *Service, home string) {
	t.Helper()
	result, err := service.Doctor(context.Background(), DoctorRequest{Home: home, Shell: "/bin/sh"})
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range result.Diagnostics {
		if finding.Severity == "error" {
			t.Fatalf("doctor rejected compiled result: %+v", result)
		}
	}
}

func TestA6DedupInstallReportsAndRouteLifecycle(t *testing.T) {
	for _, route := range fixtureRoutes(t, []string{"codex-plugin", "codex-user", "claude-user"}) {
		for _, remove := range []string{"first", "second"} {
			t.Run(route+"/remove-"+remove, func(t *testing.T) {
				service, home, _, source, foreign := nativeInstallFixture(t, route)
				provider := "codex"
				if route == "claude-user" {
					provider = "claude-code"
				}
				writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a6Manifest("first", 50))
				request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{provider}, Routes: map[string]string{provider: route}}
				if _, err := service.Install(context.Background(), request); err != nil {
					t.Fatal(err)
				}
				second := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), a6Manifest("second", 50))
				standalone, err := service.Plan(context.Background(), PlanRequest{Source: second, Harnesses: []string{provider}})
				if err != nil || len(standalone.Reports) != 1 {
					t.Fatalf("standalone=%+v %v", standalone, err)
				}
				request.Source = second
				result, err := service.Install(context.Background(), request)
				if err != nil || !result.Changed || len(result.Reports) != 2 {
					t.Fatalf("combined=%+v %v", result, err)
				}
				a6AssertCanonical(t, home, provider, "first", 2)
				for _, report := range result.Reports {
					isAlias := report.Plan.Package == "second"
					if len(report.Plan.Rules) != 6 || len(report.Tokens.Rules) != 6 {
						t.Fatal("missing report rows")
					}
					for i, rule := range report.Plan.Rules {
						want := ""
						if isAlias && rule.Status != plan.Skipped && rule.Status != plan.Never {
							want = "first/" + rule.ID
						}
						if rule.DeduplicatedInto != want || report.Tokens.Rules[i].DeduplicatedInto != want || rule.Status != standalone.Reports[0].Plan.Rules[i].Status || report.Tokens.Rules[i].Estimate != standalone.Reports[0].Tokens.Rules[i].Estimate {
							t.Fatalf("lost status/estimate/alias: %+v", report)
						}
					}
					if isAlias && (len(report.Tokens.Events) != 0 || report.Tokens.AlwaysOn != (tokens.Estimate{}) || report.SessionCost.Total != 0) {
						t.Fatalf("alias still costs: %+v", report)
					}
					if !isAlias && (report.SessionCost != standalone.Reports[0].SessionCost || !reflect.DeepEqual(report.Tokens.Events, standalone.Reports[0].Tokens.Events)) {
						t.Fatalf("canonical cost changed: %+v", report)
					}
				}
				var text bytes.Buffer
				writePlanText(&text, PlanResult{Package: result.Package, Version: result.Version, Reports: result.Reports})
				if !strings.Contains(text.String(), "deduplicated into first/static; no additional cost") || strings.Contains(text.String(), "Shared static instructions.") {
					t.Fatalf("unsafe/incomplete report: %s", text.String())
				}
				encoded, _ := json.Marshal(result.Reports)
				if !bytes.Contains(encoded, []byte(`"deduplicated_into":"first/static"`)) || bytes.Contains(encoded, []byte("Shared prompt instructions.")) {
					t.Fatal("unsafe/incomplete JSON report")
				}
				if route == "codex-plugin" && remove == "second" {
					a6Golden(t, "dedup-install-codex.txt", text.Bytes())
					var data bytes.Buffer
					if err := encodeJSON(&data, result.Reports); err != nil {
						t.Fatal(err)
					}
					a6Golden(t, "dedup-install-codex.json", data.Bytes())
				}
				if after, err := service.Plan(context.Background(), PlanRequest{Source: second, Harnesses: []string{provider}}); err != nil || !reflect.DeepEqual(after, standalone) {
					t.Fatalf("standalone plan read installed aliases: %v", err)
				}
				for _, name := range []string{"first", "second"} {
					if _, err := os.Stat(filepath.Join(home, "packages", name, "STYLE.yaml")); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(approvalPath(home, name)); err != nil {
						t.Fatal(err)
					}
				}
				a6AssertDoctor(t, service, home)
				// Route switching consumes the same effective index and static text.
				if provider == "codex" {
					for _, selected := range fixtureRoutes(t, []string{"codex-user", "codex-plugin"}) {
						request.Routes[provider] = selected
						if _, err := service.Install(context.Background(), request); err != nil {
							t.Fatal(err)
						}
						a6AssertCanonical(t, home, provider, "first", 2)
						a6AssertDoctor(t, service, home)
					}
				}
				if repeat, err := service.Install(context.Background(), request); err != nil || repeat.Changed {
					t.Fatalf("repeat install=%+v %v", repeat, err)
				}
				if synced, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || synced.Changed || len(synced.NeedsTrust) != 0 {
					t.Fatalf("idempotent sync=%+v %v", synced, err)
				}
				if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: remove, Yes: true}); err != nil {
					t.Fatal(err)
				}
				remaining := "first"
				if remove == "first" {
					remaining = "second"
				}
				a6AssertCanonical(t, home, provider, remaining, 1)
				a6AssertDoctor(t, service, home)
				if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
					t.Fatal(err)
				}
				for path, want := range foreign {
					info, err := os.Stat(path)
					if err != nil || info.Mode().Perm() != fixtureMode(0o640, false) || !bytes.Equal(readSyncFile(t, path), want) {
						t.Fatalf("foreign file not restored: %s %v", path, err)
					}
				}
			})
		}
	}
}

func a6Golden(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join(repositoryRoot(t), "testdata", "golden", name)
	if os.Getenv("STYLE_UPDATE_GOLDENS") == "1" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(data, readSyncFile(t, path)) {
		t.Fatalf("report differs from %s", name)
	}
}

func TestA6DedupTrustAndDevelopmentLinkRecomputeCanonical(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(fmt.Sprintf("link=%t", link), func(t *testing.T) {
			service, home, config, source, _ := nativeInstallFixture(t, "codex-plugin")
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a6Manifest("first", 50))
			request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{"codex"}, Link: link}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			second := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), a6Manifest("second", 50))
			request.Source = second
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if link {
				for _, name := range []string{"first", "second"} {
					if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: name, Yes: true}); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				second = filepath.Join(home, "packages", "second")
			}
			a6AssertCanonical(t, home, "codex", "first", 2)
			for _, change := range []struct {
				priority  float64
				body      string
				canonical string
			}{
				{49.5, "", "second"}, {49.5, "Changed static instructions.", ""}, {51, "", "first"},
			} {
				manifest := a6Manifest("second", change.priority)
				if change.body != "" {
					manifest = strings.ReplaceAll(manifest, "Shared static instructions.", change.body)
				}
				writeSyncFile(t, filepath.Join(second, "STYLE.yaml"), manifest)
				before := installedTree(t, home, config)
				synced, err := service.Sync(context.Background(), SyncRequest{Home: home})
				if err != nil {
					t.Fatal(err)
				}
				if !link {
					if !reflect.DeepEqual(synced.NeedsTrust, []string{"second"}) || !reflect.DeepEqual(before, installedTree(t, home, config)) {
						t.Fatal("ordinary change bypassed trust")
					}
					preview, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "second"})
					if err != nil || !preview.AwaitingConfirmation || len(preview.Reports) < 2 || !reflect.DeepEqual(before, installedTree(t, home, config)) {
						t.Fatalf("trust preview=%+v %v", preview, err)
					}
					if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "second", Yes: true}); err != nil {
						t.Fatal(err)
					}
				} else if !synced.Changed || len(synced.NeedsTrust) != 0 {
					t.Fatalf("trusted link did not sync: %+v", synced)
				}
				if change.canonical != "" {
					a6AssertCanonical(t, home, "codex", change.canonical, 2)
				} else if len(a6Index(t, home).Static["codex"]) != 2 {
					t.Fatal("changed body was still folded")
				}
				a6AssertDoctor(t, service, home)
			}
		})
	}
}

func a6Engine(t *testing.T, home string, store *state.Store) *engine.Engine {
	t.Helper()
	loaded, err := engine.LoadIndexBytes(readSyncFile(t, filepath.Join(home, "build", "index.json")), store)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func a6Input(scope state.Scope, point string) engine.Input {
	return engine.Input{Harness: "codex", Source: "startup", TurnID: "one", Scope: scope, Variables: map[string]string{"model": "X"}, Points: []engine.Point{{Name: point, CanonicalTool: "shell", ToolName: "Bash", Capabilities: engine.Capabilities{AddContext: true, SystemMessage: true, Deny: true}}}}
}

func TestA6DedupRuntimeRepeatConcurrencyLifecycleAndCanonicalHandoff(t *testing.T) {
	service, home, _, source, _ := nativeInstallFixture(t, "codex-plugin")
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a6Manifest("first", 50))
	request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{"codex"}}
	if _, err := service.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Source = writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), a6Manifest("second", 50))
	if _, err := service.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	store, err := state.New(filepath.Join(home, "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("a6-session", "main", state.EventUpdate{Source: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	loaded := a6Engine(t, home, store)
	assertRecords := func(output engine.Output) {
		t.Helper()
		for _, record := range output.Records {
			if !strings.HasPrefix(record.Entry, "first/") {
				t.Fatalf("alias generated telemetry: %+v", record)
			}
		}
	}
	var wg sync.WaitGroup
	outputs := make(chan engine.Output, 12)
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			output, err := loaded.Process(a6Input(scope, plan.SessionStart), tokens.InputLimit{})
			outputs <- output
			errors <- err
		}()
	}
	wg.Wait()
	close(outputs)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	emits, dedups := 0, 0
	for output := range outputs {
		assertRecords(output)
		if output.AdditionalContext == "Model X." {
			emits++
		} else if output.AdditionalContext != "" {
			t.Fatalf("repeated text: %+v", output)
		}
		for _, record := range output.Records {
			if record.Result == "dedup" {
				dedups++
			}
		}
	}
	if emits != 1 || dedups != 11 {
		t.Fatalf("concurrent emits=%d dedup=%d", emits, dedups)
	}
	for _, source := range []string{"compact", "clear"} {
		_, scope, err = store.Update("a6-session", "main", state.EventUpdate{Source: source})
		if err != nil {
			t.Fatal(err)
		}
		input := a6Input(scope, plan.SessionStart)
		input.Source = source
		output, err := loaded.Process(input, tokens.InputLimit{})
		if err != nil || output.AdditionalContext != "Model X." {
			t.Fatalf("%s=%+v %v", source, output, err)
		}
		assertRecords(output)
	}
	// Cap-drop must release the canonical claim so a later legal call can emit.
	input := a6Input(scope, plan.PromptSubmitted)
	output, err := loaded.Process(input, tokens.InputLimit{Value: 1, Unit: tokens.TokensUnit})
	if err != nil || output.AdditionalContext != "" {
		t.Fatalf("cap=%+v %v", output, err)
	}
	assertRecords(output)
	foundDrop := false
	for _, record := range output.Records {
		foundDrop = foundDrop || record.Result == "cap-drop"
	}
	if !foundDrop {
		t.Fatal("missing cap-drop")
	}
	output, err = loaded.Process(input, tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "Shared prompt instructions." {
		t.Fatalf("cap reservation not released: %+v %v", output, err)
	}
	for i := 0; i < 2; i++ {
		warn, err := loaded.Process(a6Input(scope, plan.ToolPost), tokens.InputLimit{})
		if err != nil || warn.AdditionalContext != "Warning." || warn.SystemMessage != "Warning. More detail." || len(warn.Reservations) != 0 {
			t.Fatalf("warn=%+v %v", warn, err)
		}
		assertRecords(warn)
		blockInput := a6Input(scope, plan.ToolPre)
		blockInput.Points[0].When.Fields = mapFields{"tool.input.command": "DANGER"}
		block, err := loaded.Process(blockInput, tokens.InputLimit{})
		if err != nil || !block.Block || block.Reason != "Shared denial." || len(block.Reservations) != 0 {
			t.Fatalf("block=%+v %v", block, err)
		}
		assertRecords(block)
		childInput := a6Input(scope, plan.SubagentStart)
		childInput.Scope.AgentID = "child"
		child, err := loaded.Process(childInput, tokens.InputLimit{})
		if err != nil || child.AdditionalContext != "Shared child instructions." || len(child.Reservations) != 0 {
			t.Fatalf("child=%+v %v", child, err)
		}
		assertRecords(child)
	}
	if unblocked, err := loaded.Process(a6Input(scope, plan.ToolPre), tokens.InputLimit{}); err != nil || unblocked.Block {
		t.Fatal("unmatched guard blocked")
	}
	// Existing repeat state belongs to first/rule. Removing that package lets
	// second/rule emit once in the same session, as disclosed by D3.
	if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "first", Yes: true}); err != nil {
		t.Fatal(err)
	}
	loaded = a6Engine(t, home, store)
	output, err = loaded.Process(a6Input(scope, plan.PromptSubmitted), tokens.InputLimit{})
	if err != nil || !strings.Contains(output.AdditionalContext, "Shared prompt instructions.") {
		t.Fatalf("handoff=%+v %v", output, err)
	}
	for _, record := range output.Records {
		if !strings.HasPrefix(record.Entry, "second/") {
			t.Fatalf("stale canonical: %+v", record)
		}
	}
	output, err = loaded.Process(a6Input(scope, plan.PromptSubmitted), tokens.InputLimit{})
	if err != nil || output.AdditionalContext != "" {
		t.Fatalf("handoff repeat=%+v %v", output, err)
	}
}

type mapFields map[string]string

func (fields mapFields) ReadConditionField(key string) (string, bool) {
	value, ok := fields[key]
	return value, ok
}

func a6LongManifest(name, body, extra, target string) string {
	selector := "    tool: shell\n    at: before\n"
	if target == "system" {
		selector = ""
	}
	return "spec: style/v0.3\nname: " + name + "\nversion: 0.1.0\ndescription: Dedup budget fixture\n" + extra + "rules:\n  - id: r\n    target: " + target + "\n" + selector + "    text: '" + body + "'\n"
}

func TestA6DedupBudgetUsesEffectiveOutputAndRejectsRealGrowth(t *testing.T) {
	for _, scenario := range []string{"identical", "different repeat", "runtime coincidence"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
			service := testCodexInstallService(t, config, filepath.Join(home, "adapters", "codex", "plugin"))
			t.Setenv("STYLE_HOME", home)
			body := strings.Repeat("a", 4008)
			first := writeInstallPackage(t, filepath.Join(root, "first"), a6LongManifest("first", body, "", "tool"))
			secondBody := body
			if scenario == "runtime coincidence" {
				secondBody = "{{model}}" + strings.Repeat("a", 4008-64)
			}
			secondManifest := a6LongManifest("second", secondBody, "", "tool")
			if scenario == "different repeat" {
				secondManifest = strings.Replace(secondManifest, "    text:", "    repeat: every\n    text:", 1)
			}
			second := writeInstallPackage(t, filepath.Join(root, "second"), secondManifest)
			for _, source := range []string{first, second} {
				planned, err := service.Plan(context.Background(), PlanRequest{Source: source, Harnesses: []string{"codex"}})
				if err != nil || len(planned.Reports) != 1 || len(planned.Reports[0].Tokens.Events) != 1 || planned.Reports[0].Tokens.Events[0].Estimate.Tokens != 1002 {
					t.Fatalf("source cost=%+v %v", planned, err)
				}
			}
			if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: first, Yes: true, Harnesses: []string{"codex"}}); err != nil {
				t.Fatal(err)
			}
			before := installedTree(t, home, config)
			var out, errOut bytes.Buffer
			code := RunWithServiceInput(context.Background(), service, root, []string{"install", second, "--harness=codex", "--json"}, approve(), &out, &errOut)
			if scenario == "identical" {
				var result InstallResult
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatalf("decode %s: %v", out.String(), err)
				}
				if code != ExitOK || !strings.HasSuffix(errOut.String(), "[y/N] \n") || len(result.Reports) != 2 || result.Reports[1].Tokens.Rules[0].DeduplicatedInto != "first/r" || len(result.Reports[1].Tokens.Events) != 0 {
					t.Fatalf("effective legal install: %d %s %s", code, out.String(), errOut.String())
				}
				if entries := a6Index(t, home).Harnesses["codex"][plan.ToolPre]; len(entries) != 1 || entries[0].Rule != "first/r" {
					t.Fatalf("duplicate runtime output: %+v", entries)
				}
			} else {
				if code != ExitBlocked || !strings.Contains(errOut.String(), "T003") || !reflect.DeepEqual(before, installedTree(t, home, config)) {
					t.Fatalf("real growth not rejected atomically: %d %s %s", code, out.String(), errOut.String())
				}
			}
		})
	}
}

func TestA6DedupCannotBypassSourceBudgets(t *testing.T) {
	for _, test := range []struct{ code, budget, target string }{
		{"T001", "per_rule: 1", "tool"},
		{"T002", "per_event: 1", "tool"},
		{"T004", "always_on: 1", "system"},
	} {
		t.Run(test.code, func(t *testing.T) {
			root := t.TempDir()
			home, config := filepath.Join(root, "style"), filepath.Join(root, "config")
			service := testCodexInstallService(t, config, filepath.Join(home, "adapters", "codex", "plugin"))
			t.Setenv("STYLE_HOME", home)
			body := "A sufficiently long identical instruction."
			first := writeInstallPackage(t, filepath.Join(root, "first"), a6LongManifest("first", body, "", test.target))
			second := writeInstallPackage(t, filepath.Join(root, "second"), a6LongManifest("second", body, "budget: {"+test.budget+", on: error}\n", test.target))
			if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: first, Yes: true, Harnesses: []string{"codex"}}); err != nil {
				t.Fatal(err)
			}
			before := installedTree(t, home, config)
			var out, errOut bytes.Buffer
			code := RunWithServiceInput(context.Background(), service, root, []string{"install", second, "--harness=codex"}, approve(), &out, &errOut)
			wantCode, diagnosticText := ExitBlocked, errOut.String()
			if test.code == "T001" {
				// Static T001 is already a source validation error before planning.
				wantCode, diagnosticText = ExitUsage, out.String()
			} else if !strings.Contains(diagnosticText, "single-package second") {
				t.Fatalf("source budget lost its scope: %s", diagnosticText)
			}
			if code != wantCode || !strings.Contains(diagnosticText, test.code) || !reflect.DeepEqual(before, installedTree(t, home, config)) {
				t.Fatalf("source budget bypassed: %d %s %s", code, out.String(), errOut.String())
			}
		})
	}
}

func TestA6DedupAmbiguityReportedAndNativeCommandsPreserved(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprintf("ambiguous=%t", ambiguous), func(t *testing.T) {
			service, home, config, source, _ := nativeInstallFixture(t, "codex-user")
			manifest := nativePackageYAML("first")
			if ambiguous {
				manifest = strings.Replace(manifest, "native_hooks:\n", "  - {id: duplicate, target: tool, tool: shell, at: before, text: Be careful.}\nnative_hooks:\n", 1)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
			request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			request.Source = writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), nativePackageYAML("second"))
			result, err := service.Install(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			assertNativeInstalled(t, home, config, "codex-user", "first", "second")
			wantCount := 1
			if ambiguous {
				wantCount = 3
			}
			if count := len(a6Index(t, home).Harnesses["codex"][plan.ToolPre]); count != wantCount {
				t.Fatalf("runtime count=%d want=%d", count, wantCount)
			}
			for _, report := range result.Reports {
				for _, rule := range report.Plan.Rules {
					if ambiguous {
						if rule.DeduplicatedInto != "" || !strings.Contains(strings.Join(rule.Reasons, "\n"), "multiple equivalent rules in one package") {
							t.Fatalf("ambiguous rule lacks explanation: %+v", rule)
						}
					}
				}
			}
		})
	}
}

func TestA6DedupTurnAndRenderedBodyClaimsStayIndependent(t *testing.T) {
	service, home, _, source, _ := nativeInstallFixture(t, "codex-plugin")
	for _, name := range []string{"first", "second"} {
		manifest := strings.Replace(a6Manifest(name, 50), "event: prompt.submitted, text:", "event: prompt.submitted, repeat: turn, text:", 1)
		path := source
		if name == "second" {
			path = filepath.Join(filepath.Dir(source), "second")
		}
		path = writeInstallPackage(t, path, manifest)
		if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: path, Yes: true, Harnesses: []string{"codex"}}); err != nil {
			t.Fatal(err)
		}
	}
	store, err := state.New(filepath.Join(home, "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, scope, err := store.Update("turn-session", "main", state.EventUpdate{Source: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	loaded := a6Engine(t, home, store)
	for _, test := range []struct{ turn, model, want string }{
		{"one", "X", "Model X.\n\nShared prompt instructions."},
		{"one", "X", ""},
		{"two", "X", "Shared prompt instructions."},
		{"two", "Y", "Model Y."},
		{"two", "Y", ""},
	} {
		input := a6Input(scope, plan.PromptSubmitted)
		input.TurnID, input.Variables["model"] = test.turn, test.model
		output, err := loaded.Process(input, tokens.InputLimit{})
		if err != nil || output.AdditionalContext != test.want {
			t.Fatalf("turn=%s model=%s output=%+v %v", test.turn, test.model, output, err)
		}
		for _, record := range output.Records {
			if !strings.HasPrefix(record.Entry, "first/") {
				t.Fatalf("alias claim/telemetry: %+v", record)
			}
		}
	}
}
