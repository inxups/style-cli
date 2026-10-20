package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

type catalogDetector struct {
	id        adapter.ProviderID
	installed bool
	calls     *atomic.Int64
}

// Compilation needs only the route name for its report. Any route operation
// during this offline test would panic through the absent embedded route.
type catalogReportRoute struct{ adapter.Route }

func (catalogReportRoute) Name() string { return "fixture-route" }

func (detector catalogDetector) Detect(context.Context) (adapter.Installation, bool, error) {
	if detector.calls != nil {
		detector.calls.Add(1)
	}
	return adapter.Installation{Provider: detector.id, Version: "1.2.3"}, detector.installed, nil
}

func catalogDefinition(id adapter.ProviderID, installed bool, calls *atomic.Int64) adapter.Definition {
	// Reuse facts as a test double, but supply a different registration ID,
	// independent detection and no native capability or writable route.
	definition := codex.Definition()
	definition.ID = id
	definition.Detector = catalogDetector{id: id, installed: installed, calls: calls}
	definition.Native = nil
	definition.Routes = nil
	return definition
}

func catalogSource(t *testing.T, root, name, harness, position, format string) string {
	t.Helper()
	header := "spec: style/v0.3\nname: " + name + "\nversion: 1.0.0\ndescription: Registry admission fixture\n"
	when := ""
	if position == "requires" || position == "all" {
		header += fmt.Sprintf("requires: {harness: {%q: '>=1.0.0'}}\n", harness)
	}
	if position == "recommends" || position == "all" {
		header += fmt.Sprintf("recommends: {%q: {effort: high}}\n", harness)
	}
	if position == "when" || position == "all" {
		when = fmt.Sprintf("when: {harness: {name: %q}}\n", harness)
	}
	if format == "single" {
		path := filepath.Join(root, name+".style.md")
		writeSyncFile(t, path, "---\n"+header+"target: system\n"+when+"---\nbody\n")
		return path
	}
	if format == "from" {
		source := writeInstallPackage(t, root, header+"rules:\n  - from: rules/r.md\n    "+when)
		writeSyncFile(t, filepath.Join(source, "rules", "r.md"), "---\ntarget: system\nwhen: {harness: {name: replaced-harness}}\n---\nbody\n")
		return source
	}
	return writeInstallPackage(t, root, header+"rules:\n  - {id: base, target: system, text: Shared base.}\n  - id: r\n    target: system\n    text: body\n    "+when)
}

func TestM3A0RegisteredNamesAreIndependentOfDetectionAndSelection(t *testing.T) {
	for _, format := range []string{"directory", "from", "single"} {
		t.Run(format, func(t *testing.T) {
			var calls atomic.Int64
			service, err := New(catalogDefinition("fixture-harness", false, &calls))
			if err != nil {
				t.Fatal(err)
			}
			source := catalogSource(t, t.TempDir(), "registered", "fixture-harness", "all", format)
			validation, err := service.Validate(context.Background(), ValidateRequest{Source: source})
			if err != nil || !validation.Valid() || calls.Load() != 0 {
				t.Fatalf("offline validation = %+v, %v; detectors called %d times", validation, err, calls.Load())
			}
			planned, err := service.Plan(context.Background(), PlanRequest{Source: source, Harnesses: []string{"fixture-harness"}})
			if err != nil || hasErrors(planned.Diagnostics) || len(planned.Reports) != 1 || planned.Reports[0].Provider != "fixture-harness" || planned.Reports[0].Plan.Blocked {
				t.Fatalf("explicit undetected fake plan = %+v, %v", planned, err)
			}
			// With an installed, selected Codex adapter, the unselected fake
			// remains a known package name.
			service, err = New(catalogDefinition("codex", true, nil), catalogDefinition("fixture-harness", false, nil))
			if err != nil {
				t.Fatal(err)
			}
			planned, err = service.Plan(context.Background(), PlanRequest{Source: source, Harnesses: []string{"codex"}})
			if err != nil || hasErrors(planned.Diagnostics) || len(planned.Reports) != 1 || planned.Reports[0].Provider != "codex" {
				t.Fatalf("unselected registered fake was rejected: %+v, %v", planned, err)
			}
		})
	}
}

func TestM3A0AutoAndBudgetEnumerationIncludeEveryRegistration(t *testing.T) {
	var calls atomic.Int64
	service, err := New(
		catalogDefinition("z-fixture", true, &calls), catalogDefinition("claude-code", true, &calls),
		catalogDefinition("a-fixture", true, &calls), catalogDefinition("codex", true, &calls),
	)
	if err != nil {
		t.Fatal(err)
	}
	source := writeInstallPackage(t, t.TempDir(), "spec: style/v0.3\nname: budget-fixture\nversion: 1.0.0\ndescription: Budget fixture\nbudget: {per_rule: 1, per_event: 1, always_on: 1, on: warn}\nrules:\n  - {id: static, target: system, text: 'Long static body for budget.'}\n  - {id: prompt, target: event, event: prompt.submitted, text: 'Long prompt body for budget.'}\n")
	validation, err := service.Validate(context.Background(), ValidateRequest{Source: source})
	if err != nil || !validation.Valid() || calls.Load() != 0 {
		t.Fatalf("budget validation = %+v, %v; detector calls=%d", validation, err, calls.Load())
	}
	for _, provider := range []string{"codex", "claude-code", "a-fixture", "z-fixture"} {
		for _, code := range []string{"T002", "T004"} {
			found := false
			for _, diagnostic := range validation.Diagnostics {
				found = found || diagnostic.Provider == provider && diagnostic.Code == code
			}
			if !found {
				t.Fatalf("%s budget %s was not evaluated: %+v", provider, code, validation.Diagnostics)
			}
		}
	}
	for _, selection := range [][]string{nil, {"auto"}, {"z-fixture,codex,a-fixture,claude"}} {
		planned, err := service.Plan(context.Background(), PlanRequest{Source: source, Harnesses: selection})
		if err != nil || hasErrors(planned.Diagnostics) {
			t.Fatalf("plan %v = %+v, %v", selection, planned, err)
		}
		var got []string
		for _, report := range planned.Reports {
			got = append(got, report.Provider)
		}
		if want := []string{"codex", "claude-code", "a-fixture", "z-fixture"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("selection %v report order = %v, want %v", selection, got, want)
		}
	}
}

func TestM3A0ServiceSnapshotsAndConcurrentValidation(t *testing.T) {
	var calls atomic.Int64
	definition := catalogDefinition("fixture-harness", false, &calls)
	first, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.ID = "other-harness"
	second, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	source := catalogSource(t, t.TempDir(), "registered", "fixture-harness", "all", "directory")
	pkg, err := spec.Load(source)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for index := range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			service := first
			if index%2 != 0 {
				service = second
			}
			for range 10 {
				got := service.validateSpec(pkg)
				if index%2 == 0 && len(got) != 0 || index%2 != 0 && len(got) != 3 {
					t.Errorf("service catalog contamination: %+v", got)
					return
				}
			}
		}()
	}
	workers.Wait()
	if calls.Load() != 0 || !reflect.DeepEqual(first.catalog.Harnesses, []string{"fixture-harness"}) || !reflect.DeepEqual(second.catalog.Harnesses, []string{"other-harness"}) {
		t.Fatal("constructing or validating a catalog detected a harness or borrowed caller data")
	}
	empty, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.validateSpec(pkg); len(got) != 3 {
		t.Fatalf("empty Service admitted names: %+v", got)
	}
}

func TestM3A0ValidationNeverDetectsOrExecutesNativeCommands(t *testing.T) {
	var calls atomic.Int64
	definition := codex.Definition()
	definition.Detector = catalogDetector{id: "codex", installed: false, calls: &calls}
	service, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	marker := filepath.Join(root, "must-not-exist")
	manifest := fmt.Sprintf("spec: style/v0.3\nname: native-validation\nversion: 1.0.0\ndescription: Native validation fixture\nnative_hooks:\n  - id: native\n    codex: {event: Stop, command: %q}\n", "touch "+marker)
	source := writeInstallPackage(t, filepath.Join(root, "source"), manifest)
	before := snapshotStatsTree(t, root)
	result, err := service.Validate(context.Background(), ValidateRequest{Source: source})
	if err != nil || !result.Valid() || calls.Load() != 0 {
		t.Fatalf("native validation = %+v, %v; detector calls=%d", result, err, calls.Load())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) || !reflect.DeepEqual(before, snapshotStatsTree(t, root)) {
		t.Fatal("validation executed a native command or wrote files")
	}
}

func TestM3A0AdmissionFailuresPreserveInstalledBytesModesAndTimes(t *testing.T) {
	for _, position := range []string{"when", "requires", "recommends"} {
		for _, operation := range []string{"validate", "plan", "new-directory", "new-single", "new-link", "reinstall", "install-retained", "sync", "trust-preview", "trust-yes", "uninstall-retained"} {
			t.Run(position+"/"+operation, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
				limited := testCodexInstallService(t, config, filepath.Join(home, "adapters", "codex", "plugin"))
				full, err := New(limited.definitions["codex"], catalogDefinition("fixture-harness", false, nil))
				if err != nil {
					t.Fatal(err)
				}
				source := catalogSource(t, filepath.Join(root, "guarded"), "guarded", "fixture-harness", position, "directory")
				request := InstallRequest{Home: home, Source: source, Harnesses: []string{"codex"}, Yes: true}
				if result, err := full.Install(ctx, request); err != nil || !result.Changed {
					t.Fatalf("fixture install = %+v, %v", result, err)
				}
				other := catalogSource(t, filepath.Join(root, "other"), "other", "", "", "directory")
				if _, err := full.Install(ctx, InstallRequest{Home: home, Source: other, Harnesses: []string{"codex"}, Yes: true}); err != nil {
					t.Fatal(err)
				}
				writeSyncFile(t, filepath.Join(home, "state", "sentinel"), "state must be preserved")
				format := "directory"
				if operation == "new-single" {
					format = "single"
				}
				candidate := catalogSource(t, filepath.Join(root, "candidate"), "candidate", "fixture-harness", position, format)
				if operation == "install-retained" {
					candidate = catalogSource(t, filepath.Join(root, "valid"), "candidate", "", "", "directory")
				}
				before := snapshotStatsTree(t, root)
				var diagnostics []Diagnostic
				switch operation {
				case "validate":
					result, validateErr := limited.Validate(ctx, ValidateRequest{Source: source})
					diagnostics, err = result.Diagnostics, validateErr
				case "plan":
					result, planErr := limited.Plan(ctx, PlanRequest{Source: source, Harnesses: []string{"codex"}})
					diagnostics, err = result.Diagnostics, planErr
				case "new-directory", "new-single", "new-link", "install-retained":
					request.Source, request.Link = candidate, operation == "new-link"
					_, err = limited.Install(ctx, request)
				case "reinstall":
					_, err = limited.Install(ctx, request)
				case "sync":
					_, err = limited.Sync(ctx, SyncRequest{Home: home})
				case "trust-preview", "trust-yes":
					_, err = limited.Trust(ctx, TrustRequest{Home: home, Name: "guarded", Yes: operation == "trust-yes"})
				case "uninstall-retained":
					_, err = limited.Uninstall(ctx, UninstallRequest{Home: home, Name: "other", Yes: true})
				}
				if operation != "validate" && operation != "plan" {
					var invalid *ValidationError
					if !errors.As(err, &invalid) {
						t.Fatalf("expected admission ValidationError, got %v", err)
					}
					diagnostics, err = invalid.Diagnostics, nil
				}
				messages := map[string]string{
					"when": `unknown harness name "fixture-harness"`, "requires": `unknown harness "fixture-harness"`, "recommends": `unknown recommends harness "fixture-harness"`,
				}
				if err != nil || len(diagnostics) != 1 || diagnostics[0].Code != "invalid-schema" || diagnostics[0].Severity != spec.SeverityError || diagnostics[0].Message != messages[position] || diagnostics[0].Path == "" {
					t.Fatalf("inconsistent name diagnostic: %+v, %v", diagnostics, err)
				}
				if after := snapshotStatsTree(t, root); !reflect.DeepEqual(before, after) {
					t.Fatal("rejected candidate changed package, lock, index, approval, state or external configuration bytes/modes/mtimes")
				}
			})
		}
	}
}

func TestM3A0RegistryChangeRecoveryAndRemovedPackageCompatibility(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
	limited := testCodexInstallService(t, config, filepath.Join(home, "adapters", "codex", "plugin"))
	full, err := New(limited.definitions["codex"], catalogDefinition("fixture-harness", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "config.toml")
	original := "# User bytes must survive\nmodel = 'external'\n"
	writeSyncFile(t, path, original)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"guarded", "other"} {
		position := "all"
		if name == "other" {
			position = ""
		}
		source := catalogSource(t, filepath.Join(root, name), name, "fixture-harness", position, "directory")
		if _, err := full.Install(ctx, InstallRequest{Home: home, Source: source, Harnesses: []string{"codex"}, Yes: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := limited.Sync(ctx, SyncRequest{Home: home}); err == nil {
		t.Fatal("removed registration must invalidate the retained package")
	}
	if _, err := full.Sync(ctx, SyncRequest{Home: home}); err != nil {
		t.Fatalf("restoring registration did not recover: %v", err)
	}
	// Do not readmit the package being removed. Only remaining packages
	// require the new catalog, preserving a way to repair an old install.
	if result, err := limited.Uninstall(ctx, UninstallRequest{Home: home, Name: "guarded", Yes: true}); err != nil || !reflect.DeepEqual(result.Remaining, []string{"other"}) {
		t.Fatalf("removing invalid old package = %+v, %v", result, err)
	}
	if _, err := limited.Uninstall(ctx, UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != fixtureMode(0o640, false) || string(readSyncFile(t, path)) != original {
		t.Fatal("uninstall did not restore external bytes/mode")
	}
}

func TestM3A0FakeBudgetDiagnosticsSurviveCompileAndDedup(t *testing.T) {
	ctx := context.Background()
	service, err := New(catalogDefinition("fixture-harness", true, nil))
	if err != nil {
		t.Fatal(err)
	}
	var packages []installPackage
	for index, name := range []string{"first", "second"} {
		manifest := "spec: style/v0.3\nname: " + name + "\nversion: 1.0.0\ndescription: Catalog budget fixture\nbudget: {per_rule: 1, per_event: 1, on: warn}\nrules:\n  - {id: r, target: event, event: prompt.submitted, repeat: turn, text: 'Shared budget instruction.', when: {harness: {name: fixture-harness}}}\n"
		source := writeInstallPackage(t, t.TempDir(), manifest)
		validation, err := service.Validate(ctx, ValidateRequest{Source: source})
		if err != nil || !validation.Valid() {
			t.Fatalf("fake validation = %+v, %v", validation, err)
		}
		resolved, err := spec.Merge(validation.Package)
		if err != nil {
			t.Fatal(err)
		}
		packages = append(packages, installPackage{lock: lockPackage{Name: name, Seq: index + 1, Files: map[string]string{}}, pkg: validation.Package, resolved: resolved, source: validation.source,
			report: map[adapter.ProviderID]plan.Report{}, tokens: map[adapter.ProviderID]tokens.Report{}})
	}
	target := installAdapter{provider: "fixture-harness", definition: service.definitions["fixture-harness"], installation: adapter.Installation{Provider: "fixture-harness", Version: "1.2.3"}, route: catalogReportRoute{}}
	compiled, err := service.compileInstalled(ctx, packages, []installAdapter{target}, "second")
	if err != nil || len(compiled.reports) != 2 {
		t.Fatalf("compile/dedup fake = %+v, %v", compiled, err)
	}
	for _, code := range []string{"T001", "T007"} {
		found := false
		for _, diagnostic := range compiled.diagnostics {
			found = found || diagnostic.Code == code && diagnostic.Provider == "fixture-harness" && strings.HasPrefix(diagnostic.Message, "single-package ")
		}
		if !found {
			t.Fatalf("pre-dedup %s lost source lint: %+v", code, compiled.diagnostics)
		}
		for _, report := range compiled.reports {
			found = false
			for _, diagnostic := range report.Tokens.Diagnostics {
				found = found || diagnostic.Code == code
			}
			if !found {
				t.Fatalf("effective report %s lost %s: %+v", report.Provider, code, report.Tokens)
			}
		}
	}
	if got := packages[1].report["fixture-harness"].Rules[0].DeduplicatedInto; got != "first/r" {
		t.Fatalf("fake reports did not exercise dedup: alias=%q", got)
	}
}

func TestM3A0CLIDiagnosticsRetainJSONAndExitContract(t *testing.T) {
	service, err := New(catalogDefinition("fixture-harness", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	source := catalogSource(t, t.TempDir(), "invalid", "codex", "all", "directory")
	for _, command := range []string{"validate", "plan"} {
		var stdout, stderr bytes.Buffer
		args := []string{command, source, "--json"}
		if command == "plan" {
			args = append(args, "--harness=fixture-harness")
		}
		if code := RunWithService(context.Background(), service, repositoryRoot(t), args, &stdout, &stderr); code != ExitInvalid || !strings.HasPrefix(stderr.String(), "style "+command+": ") {
			t.Fatalf("%s exit=%d stdout=%s stderr=%s", command, code, stdout.String(), stderr.String())
		}
		var diagnostics []Diagnostic
		if command == "validate" {
			if err := decodeReport(stdout.Bytes(), &diagnostics); err != nil {
				t.Fatal(err)
			}
		} else {
			var result PlanResult
			if err := decodeReport(stdout.Bytes(), &result); err != nil || len(result.Reports) != 0 {
				t.Fatalf("plan JSON = %s, %v", stdout.String(), err)
			}
			diagnostics = result.Diagnostics
		}
		if len(diagnostics) != 3 {
			t.Fatalf("%s diagnostics = %+v", command, diagnostics)
		}
		for _, diagnostic := range diagnostics {
			if diagnostic.Code != "invalid-schema" || diagnostic.Severity != spec.SeverityError || diagnostic.Provider != "" || diagnostic.Path == "" {
				t.Fatalf("JSON diagnostic contract changed: %+v", diagnostic)
			}
		}
	}
}
