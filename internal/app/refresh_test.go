package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func a9Manifest(n int) string {
	return fmt.Sprintf("spec: style/v0.3\nname: periodic\nversion: 1.0.0\ndescription: refresh fixture\nrules:\n  - {id: r, target: tool, tool: shell, at: before, text: 'Periodic guidance.', refresh: %d, reason: periodic}\n", n)
}

func TestM3A9FiveRouteLifecycleAndSurface(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user", "codex-project", "claude-project"} {
		t.Run(route, func(t *testing.T) {
			var s *Service
			var home, config, source string
			harness := "codex"
			if strings.HasPrefix(route, "claude-") {
				harness = "claude-code"
			}
			if strings.HasSuffix(route, "-project") {
				var base, project string
				s, base, project, source = a6Setup(t)
				s = a6Scope(t, s, project, base)
				home = s.project.Home
				config = project
			} else {
				s, home, config, source, _ = nativeInstallFixture(t, route)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a9Manifest(3))
			req := InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true}
			if _, err := s.Install(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			var index compile.Index
			data := readSyncFile(t, filepath.Join(home, "build", "index.json"))
			if json.Unmarshal(data, &index) != nil || index.Format != 2 || len(index.Static[harness]) != 0 || len(index.Harnesses[harness]) != 1 {
				t.Fatal(string(data))
			}
			if len(index.Harnesses[harness]["prompt.submitted"]) != 0 {
				t.Fatal("invented prompt output")
			}
			p := a1AssertProvenance(t, home)
			if p.Harnesses[harness][0].Refresh == nil || p.Harnesses[harness][0].Refresh.Interval != 3 {
				t.Fatal(p)
			}
			hooks := hooksForProvider(adapter.ProviderID(harness), index)
			if harness == "claude-code" {
				events := map[string]bool{}
				for _, hook := range hooks {
					events[hook.NativeEvent] = true
				}
				if !events["SessionStart"] || !events["UserPromptSubmit"] || !events["PreToolUse"] || events["UserPromptExpansion"] {
					t.Fatal(hooks)
				}
			}
			if result, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed {
				t.Fatal(result, err)
			}
			if result, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil || result.Changed {
				t.Fatal(result, err)
			}
			if _, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "periodic", Yes: true}); err != nil {
				t.Fatal(err)
			}
			doctor, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
			if err != nil || !doctor.Healthy {
				t.Fatal(doctor.Diagnostics, err)
			}
			found := false
			for _, d := range doctor.Diagnostics {
				found = found || d.Code == "refresh-index-2"
			}
			if !found {
				t.Fatal(doctor.Diagnostics)
			}
			check := a5Lock(t, home)
			a5Remove(t, filepath.Join(home, "build", "index.json"), filepath.Join(home, "build", "explain.json"))
			definitions := make(map[adapter.ProviderID]adapter.Definition, len(s.definitions))
			for provider, definition := range s.definitions {
				definitions[provider] = definition
			}
			sourceFetcher := s.sources
			a5Offline(t, s)
			if result, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil || !result.Changed {
				t.Fatal(result, err)
			}
			check()
			if !bytes.Equal(data, readSyncFile(t, filepath.Join(home, "build", "index.json"))) {
				t.Fatal("frozen changed approved refresh")
			}
			s.definitions, s.sources = definitions, sourceFetcher
			peer := t.TempDir()
			writeSyncFile(t, filepath.Join(peer, "STYLE.yaml"), strings.Replace(a9Manifest(3), "name: periodic", "name: peer", 1))
			peerRequest := req
			peerRequest.Source = peer
			if _, err := s.Install(context.Background(), peerRequest); err != nil {
				t.Fatal(err)
			}
			assertRetained := func(rule string) {
				t.Helper()
				if err := json.Unmarshal(readSyncFile(t, filepath.Join(home, "build", "index.json")), &index); err != nil {
					t.Fatal(err)
				}
				entries := index.Harnesses[harness]["tool.pre"]
				if len(entries) != 1 || entries[0].Rule != rule || entries[0].Refresh != 3 || index.Format != 2 {
					t.Fatal(entries, index.Format)
				}
				a1AssertProvenance(t, home)
			}
			assertRetained("periodic/r")
			if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "periodic", Yes: true}); err != nil {
				t.Fatal(err)
			}
			assertRetained("peer/r")
			if result, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed {
				t.Fatal(result, err)
			}
			if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			_ = config
		})
	}
}

func TestM3A9ExplainSnapshotMatchesRuntimeReadOnly(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			entry := a1Entry("r", "tool.pre", "Periodic guidance.")
			entry.Refresh = 3
			s, request := a1Fixture(t, harness, "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
			index := compile.Index{Format: 2, StyleVersion: "0.3.0", Packages: []compile.PackageInfo{}, Static: map[string][]compile.StaticEntry{harness: {}}, Harnesses: map[string]map[string][]compile.Entry{harness: {"tool.pre": {entry}}}}
			data, _ := compile.Encode(index)
			writeSyncFile(t, request.Index, string(data))
			request.StateDir = t.TempDir()
			store, _ := state.New(request.StateDir)
			ce := codex.NewEnricher()
			ce.Offline = true
			var enricher adapter.RuntimeEnricher = ce
			if harness == "claude-code" {
				ae := claude.NewEnricher()
				ae.Environment = func() string { return "" }
				enricher = ae
			}
			core, err := engine.LoadIndexBytes(data, store)
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := styleruntime.New(core, store, enricher, agenthooks.Provider(harness), false)
			if err != nil {
				t.Fatal(err)
			}
			key := "turn_id"
			if harness == "claude-code" {
				key = "prompt_id"
			}
			run := func(payload string) string {
				t.Helper()
				var out, logs bytes.Buffer
				code := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + harness}, strings.NewReader(payload), &out, &logs)
				if code != 0 || strings.Contains(logs.String(), "level=ERROR") {
					t.Fatal(code, out.String(), logs.String())
				}
				return out.String()
			}
			initial := fmt.Sprintf(`{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"PRIVATE"},%q:"","model":"model-a"}`, key)
			run(initial)
			for i := 1; i <= 3; i++ {
				run(fmt.Sprintf(`{"session_id":"s","hook_event_name":"UserPromptSubmit",%q:%q,"model":"model-a","prompt":"PRIVATE"}`, key, fmt.Sprint(i)))
			}
			payload := fmt.Sprintf(`{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"PRIVATE"},%q:"3","model":"model-a"}`, key)
			writeSyncFile(t, request.Payload, payload)
			before := snapshotStatsTree(t, request.StateDir)
			explained, err := s.Explain(context.Background(), request)
			if err != nil || explained.Status != "ok" || len(explained.Entries) != 1 {
				t.Fatal(explained, err)
			}
			if !reflect.DeepEqual(before, snapshotStatsTree(t, request.StateDir)) {
				t.Fatal("explain changed source state")
			}
			if tr := explained.Entries[0].Refresh; tr == nil || tr.Current != 3 || tr.Reason != "due" || tr.Last == nil || *tr.Last != 0 {
				t.Fatal(tr)
			}
			wire := run(payload)
			if explained.Wire == nil || wire != explained.Wire.Stdout {
				t.Fatal(explained.Wire, wire)
			}
			again, err := s.Explain(context.Background(), request)
			if err != nil || again.Entries[0].Refresh.Reason != "not-due" {
				t.Fatal(again, err)
			}
		})
	}
}

func TestM3A9PolicyChangesApprovalAndTransactionRollback(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user", "codex-project", "claude-project"} {
		t.Run(route, func(t *testing.T) {
			var s *Service
			var home, config, source string
			harness := "codex"
			if strings.HasPrefix(route, "claude-") {
				harness = "claude-code"
			}
			if strings.HasSuffix(route, "-project") {
				var base, project string
				s, base, project, source = a6Setup(t)
				s = a6Scope(t, s, project, base)
				home = s.project.Home
				config = project
			} else {
				s, home, config, source, _ = nativeInstallFixture(t, route)
			}
			req := InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a9Manifest(3))
			if _, err := s.Install(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			baseline := map[string][]byte{}
			for _, name := range []string{"index.json", "explain.json"} {
				path := filepath.Join(home, "build", name)
				baseline[path] = readSyncFile(t, path)
			}
			before := a4State(t, home, config)
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.Replace(a9Manifest(5), "version: 1.0.0", "version: 1.0.1", 1))
			preview, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true})
			if err != nil || !preview.AwaitingConfirmation || preview.Changed {
				t.Fatal(preview, err)
			}
			a4AssertState(t, before, home, config)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fault := &a1CancelAfterBuild{Context: ctx, cancel: cancel, before: baseline}
			if result, err := s.Update(fault, UpdateRequest{Home: home, All: true, Yes: true}); !errors.Is(err, context.Canceled) || !fault.reached || result.Changed {
				t.Fatal(result, err, fault.reached)
			}
			a4AssertState(t, before, home, config)
			if _, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			assertInterval := func(n int) {
				t.Helper()
				var index compile.Index
				data := readSyncFile(t, filepath.Join(home, "build", "index.json"))
				if err := json.Unmarshal(data, &index); err != nil {
					t.Fatal(err)
				}
				if got := index.Harnesses[harness]["tool.pre"][0].Refresh; got != n {
					t.Fatal(got, n)
				}
				if n == 0 && index.Format != 1 || n > 0 && index.Format != 2 {
					t.Fatal(index.Format)
				}
				a1AssertProvenance(t, home)
			}
			assertInterval(5)
			if s.project != nil {
				declaration, err := readProjectDeclaration(s.project.Root)
				if err != nil || declaration == nil {
					t.Fatal(declaration, err)
				}
				lock, err := s.loadLock(home)
				if err != nil || lock.Project == nil || declaration.Effects == "" || lock.Project.Declaration != hashBytes(readSyncFile(t, filepath.Join(s.project.Root, projectDeclarationPath))) {
					t.Fatal(declaration, lock.Project, err)
				}
			}
			writeSyncFile(t, filepath.Join(home, "packages", "periodic", "STYLE.yaml"), strings.Replace(a9Manifest(7), "version: 1.0.0", "version: 1.0.1", 1))
			if result, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(result.NeedsTrust) != 1 {
				t.Fatal(result, err)
			}
			assertInterval(5)
			if _, err := s.Trust(context.Background(), TrustRequest{Home: home, Name: "periodic", Yes: true}); err != nil {
				t.Fatal(err)
			}
			assertInterval(7)
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.Replace(a9Manifest(0), "version: 1.0.0", "version: 1.0.2", 1))
			if _, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
			assertInterval(0)
		})
	}
}
