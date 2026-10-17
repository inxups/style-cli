package app

import (
	"bytes"
	"context"
	"encoding/json"
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

func a11Manifest() string {
	return "spec: style/v0.3\nname: service-provider\nversion: 1.0.0\ndescription: service fixture\nreads: {provider: true}\nrules:\n  - {id: r, target: event, event: tool.pre, when: {provider: [openai, anthropic, bedrock]}, text: 'Service {{provider}}'}\n"
}
func TestM3A11SixRouteProviderLifecycleAndFrozen(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user", "claude-plugin", "codex-project", "claude-project"} {
		t.Run(route, func(t *testing.T) {
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
				fixtureRoute := route
				if route == "claude-plugin" {
					fixtureRoute = "claude-user"
				}
				s, home, config, source, _ = nativeInstallFixture(t, fixtureRoute)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a11Manifest())
			result, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{harness}, Routes: map[string]string{harness: route}, Yes: true})
			if err != nil {
				t.Fatal(err)
			}
			if route == "claude-plugin" {
				a8Registration(t, result.Registration[0], config, "enabled")
			}
			data := readSyncFile(t, filepath.Join(home, "build/index.json"))
			var index compile.Index
			if err := json.Unmarshal(data, &index); err != nil || index.Format != 4 {
				t.Fatal(err, string(data))
			}
			a1AssertProvenance(t, home)
			if r, err := s.Sync(context.Background(), SyncRequest{Home: home}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			if r, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil || r.Changed {
				t.Fatal(r, err)
			}
			d, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
			if err != nil || !d.Healthy {
				t.Fatal(d.Diagnostics, err)
			}
			found := false
			for _, v := range d.Diagnostics {
				found = found || v.Code == "runtime-index-4"
			}
			if !found {
				t.Fatal(d.Diagnostics)
			}
			check := a5Lock(t, home)
			a5Remove(t, filepath.Join(home, "build/index.json"), filepath.Join(home, "build/explain.json"))
			if _, err = s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
				t.Fatal(err)
			}
			check()
			if !bytes.Equal(data, readSyncFile(t, filepath.Join(home, "build/index.json"))) {
				t.Fatal("frozen changed provider index")
			}
			if route == "claude-plugin" {
				a8Registration(t, result.Registration[0], config, "disabled")
			}
			if _, err = s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestM3A11ProviderExplainFactsAndRealCodecs(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, mode := range []string{"known", "missing", "empty", "invalid", "binding"} {
			t.Run(harness+"/"+mode, func(t *testing.T) {
				id := "custom.ID"
				if harness == "claude-code" {
					id = "bedrock"
				}
				entry := a1Entry("provider", "tool.pre", "unused")
				fallback := "missing"
				entry.Body = compile.Body{Parts: []compile.Fragment{{Text: "Service "}, {Variable: &compile.Variable{Name: "provider", Default: &fallback}}}}
				entry.When.Provider = []string{id}
				entry.Needs = []string{"provider"}
				service, request := a1Fixture(t, harness, "PreToolUse", map[string][]compile.Entry{"tool.pre": {entry}})
				var index compile.Index
				// a1Fixture intentionally emits format 1; raise before loading new semantics.
				raw := bytes.Replace(readSyncFile(t, request.Index), []byte(`"format": 1`), []byte(`"format": 4`), 1)
				if err := json.Unmarshal(raw, &index); err != nil {
					t.Fatal(err)
				}
				writeSyncFile(t, request.Index, string(raw))
				t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
				ambient := t.TempDir()
				writeSyncFile(t, filepath.Join(ambient, "config.toml"), `model_provider="ambient"`)
				t.Setenv("CODEX_HOME", ambient)
				fields := map[string]any{"model_provider": id}
				switch mode {
				case "missing":
					fields = map[string]any{}
				case "empty":
					fields["model_provider"] = ""
				case "invalid":
					fields["model_provider"] = "https://private.invalid/secret"
				}
				a1Facts(t, &request, fields)
				if mode == "binding" {
					data := readSyncFile(t, request.Facts)
					writeSyncFile(t, request.Facts, strings.ReplaceAll(string(data), explainSHA(readSyncFile(t, request.Payload)), a1Hash))
				}
				before := snapshotStatsTree(t, filepath.Dir(request.Index))
				result, err := service.Explain(context.Background(), request)
				if mode == "invalid" || mode == "binding" {
					if err == nil || result.Wire != nil {
						t.Fatal(result, err)
					}
					return
				}
				if err != nil || result.Wire == nil || result.Status != "ok" {
					t.Fatal(result, err)
				}
				if !reflect.DeepEqual(before, snapshotStatsTree(t, filepath.Dir(request.Index))) {
					t.Fatal("explain mutated input")
				}
				if strings.Contains(result.Wire.Stdout, "ambient") || strings.Contains(result.Wire.Stdout, "private.invalid") {
					t.Fatal("ambient leak")
				}
				if (mode == "known") != strings.Contains(result.Wire.Stdout, "Service "+id) {
					t.Fatal(result.Wire)
				}
				found := false
				for _, fact := range result.Facts {
					if fact.Field == "provider" {
						found = true
						if fact.Available != (mode == "known") {
							t.Fatal(fact)
						}
					}
				}
				if !found {
					t.Fatal(result.Facts)
				}
				if mode != "known" {
					return
				}
				// Run the actual native codec with independently resolved launch/environment.
				store, _ := state.New(filepath.Join(t.TempDir(), "ordinary"))
				core, err := engine.New(index, store)
				if err != nil {
					t.Fatal(err)
				}
				var enricher adapter.RuntimeEnricher
				if harness == "codex" {
					e := codex.NewEnricher()
					e.Offline = true
					e.LaunchSettings = func(string) (agenthooks.CodexLaunchSettings, bool) {
						return agenthooks.CodexLaunchSettings{ModelProvider: id}, true
					}
					enricher = e
				} else {
					e := claude.NewEnricher()
					e.Environment = func() string { return "" }
					e.ProviderEnvironment = func(key string) (string, bool) {
						if key == "CLAUDE_CODE_USE_BEDROCK" {
							return "1", true
						}
						return "", false
					}
					enricher = e
				}
				runtime, err := styleruntime.New(core, store, enricher, agenthooks.Provider(harness), false)
				if err != nil {
					t.Fatal(err)
				}
				runtime.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
				runtime.CodecStateDir = t.TempDir()
				var stdout, stderr bytes.Buffer
				exit := runtime.Run(context.Background(), []string{"agenthooks", "run", "--provider=" + harness}, bytes.NewReader(readSyncFile(t, request.Payload)), &stdout, &stderr, agenthooks.WithoutMCPResolution())
				if exit != result.Wire.ExitCode || stdout.String() != result.Wire.Stdout || stderr.String() != result.Wire.Stderr {
					t.Fatal(exit, stdout.String(), stderr.String(), result.Wire)
				}
				session, _, err := store.Read("s", "main")
				if err != nil || session.ModelProvider != id || !session.ProviderResolved {
					t.Fatal(session, err)
				}
				for _, check := range result.Entries[0].Conditions {
					if check.Field == "provider" && check.Source != "provided" {
						t.Fatal(check)
					}
				}
			})
		}
	}
}
func TestM3A11ProviderCloudBundle(t *testing.T) {
	s, _, root, source := a7Setup(t)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), a11Manifest())
	if _, err := s.Install(context.Background(), InstallRequest{Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a7Export(t, s)
	_, files, err := readCloud(root)
	if err != nil {
		t.Fatal(err)
	}
	var index compile.Index
	if err = json.Unmarshal(files[cloudDir+"/index.json"], &index); err != nil || index.Format != 4 {
		t.Fatal(index.Format, err)
	}
	// The bundle contains selectors, not ambient service endpoints.
	if strings.Contains(string(files[cloudDir+"/index.json"]), os.Getenv("ANTHROPIC_BASE_URL")) && os.Getenv("ANTHROPIC_BASE_URL") != "" {
		t.Fatal("ambient endpoint exported")
	}
}
