package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/spec"
)

func legacyNativeManifest(name string) string {
	manifest := strings.Replace(nativePackageYAML(name), "native_hooks:\n", "", 1)
	for _, id := range []string{"later", "earlier"} {
		manifest = strings.Replace(manifest, "  - id: "+id+"\n", "  - id: "+id+"\n    target: native\n", 1)
	}
	return manifest
}

// Construct authenticated historical source without retaining a legacy parser.
// The built-CLI acceptance script additionally installs with a real old binary.
func setLegacyNativeApproval(t *testing.T, home, name string) {
	t.Helper()
	lock := mustLoadNativeLock(t, home)
	for i, item := range lock.Packages {
		if item.Name != name {
			continue
		}
		baseline, available, err := loadApproval(home, item)
		if err != nil || !available {
			t.Fatalf("baseline: %v", err)
		}
		baseline.Files["STYLE.yaml"] = []byte(legacyNativeManifest(name))
		data, err := json.Marshal(baseline)
		if err != nil {
			t.Fatal(err)
		}
		writeSyncFile(t, approvalPath(home, name), string(data))
		item.Approval = hashBytes(data)
		item.Files, _ = hashPackageFiles(baseline.Files)
		lock.Packages[i] = item
	}
	if err := writeLock(home, lock); err != nil {
		t.Fatal(err)
	}
}

func TestNativeFormatRemovedManagementIsAtomic(t *testing.T) {
	requirePOSIXFixture(t, "native lifecycle uses POSIX commands")
	for _, variant := range []string{"source-and-approval", "approval-only", "source-only"} {
		t.Run(variant, func(t *testing.T) {
			service, home, config, source, _ := nativeInstallFixture(t, "codex-user")
			t.Setenv("STYLE_HOME", home)
			request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if variant != "source-only" {
				setLegacyNativeApproval(t, home, "native-test")
			}
			if variant != "approval-only" {
				writeSyncFile(t, filepath.Join(home, "packages", "native-test", "STYLE.yaml"), legacyNativeManifest("native-test"))
			}
			candidate := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "candidate"), a4Manifest("candidate", "Body."))
			before := a4State(t, home, config)
			for _, args := range [][]string{
				{"sync", "--json"}, {"trust", "native-test", "--yes", "--json"},
				{"update", "--all", "--yes", "--json"}, {"install", "--frozen", "--yes", "--json"},
				{"install", candidate, "--harness=codex", "--route=codex=codex-user", "--yes", "--json"},
			} {
				var out, errOut bytes.Buffer
				code := RunWithService(context.Background(), service, source, args, &out, &errOut)
				output := out.String() + errOut.String()
				if code != ExitInvalid || !strings.Contains(output, spec.NativeFormatRemoved) || !strings.Contains(output, "uninstall") {
					t.Fatalf("%v exit=%d stdout=%s stderr=%s", args, code, &out, &errOut)
				}
				a4AssertState(t, before, home, config)
			}
			listed, err := service.List(context.Background(), ListRequest{Home: home})
			if err != nil || len(listed.Packages) != 1 || !strings.Contains(listed.Packages[0].Error, spec.NativeFormatRemoved) {
				t.Fatalf("list: %+v %v", listed, err)
			}
			doctor, err := service.Doctor(context.Background(), DoctorRequest{Home: home})
			found := false
			for _, d := range doctor.Diagnostics {
				found = found || d.Code == spec.NativeFormatRemoved && d.Severity == "error"
			}
			if err != nil || !found {
				t.Fatalf("doctor: %+v %v", doctor, err)
			}
			a4AssertState(t, before, home, config)
			if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			assertNativeInstalled(t, home, config, "codex-user", "native-test")
		})
	}
}

func TestLegacyNativeUninstallOnlyParsesRemainingPackages(t *testing.T) {
	requirePOSIXFixture(t, "native lifecycle uses POSIX commands")
	for _, oldRemaining := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary remains", true: "old native remains"}[oldRemaining], func(t *testing.T) {
			service, home, config, source, _ := nativeInstallFixture(t, "codex-user")
			request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			next := a4Manifest("remaining", "Body.")
			if oldRemaining {
				next = nativePackageYAML("remaining")
			}
			request.Source = writeInstallPackage(t, filepath.Join(filepath.Dir(source), "remaining"), next)
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			setLegacyNativeApproval(t, home, "native-test")
			writeSyncFile(t, filepath.Join(home, "packages/native-test/STYLE.yaml"), legacyNativeManifest("native-test"))
			if oldRemaining {
				setLegacyNativeApproval(t, home, "remaining")
				writeSyncFile(t, filepath.Join(home, "packages/remaining/STYLE.yaml"), legacyNativeManifest("remaining"))
			}
			before := a4State(t, home, config)
			result, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "native-test", Yes: true})
			if oldRemaining {
				if spec.ErrorCode(err) != spec.NativeFormatRemoved || !strings.Contains(err.Error(), "uninstall --all") {
					t.Fatalf("%+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
					t.Fatal(err)
				}
			} else if err != nil || !reflect.DeepEqual(result.Remaining, []string{"remaining"}) {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestNativeHookDescriptionEditsDoNotRequireExecutionApproval(t *testing.T) {
	requirePOSIXFixture(t, "native lifecycle uses POSIX commands")
	service, home, config, source, _ := nativeInstallFixture(t, "codex-user")
	request := InstallRequest{Home: home, Source: source, Link: true, Yes: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}
	if _, err := service.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
		t.Fatal(err)
	}
	hooks := readSyncFile(t, filepath.Join(config, "hooks.json"))
	changed := strings.Replace(nativePackageYAML("native-test"), "  - id: later\n", "  - id: later\n    description: '{{ plain notes, not a template'\n", 1)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), changed)
	result, err := service.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || !result.Changed || len(result.NeedsTrust) != 0 || len(result.Changes) != 1 || result.Changes[0].NativeChanged {
		t.Fatalf("%+v %v", result, err)
	}
	if !bytes.Equal(hooks, readSyncFile(t, filepath.Join(config, "hooks.json"))) {
		t.Fatal("description changed installed commands")
	}
	// A mapping for a harness absent from this installation is still executable source.
	changed = strings.Replace(changed, "claude-code: {event: PreToolUse, matcher: '*', command: 'printf earlier'}", "claude-code: {event: PreToolUse, matcher: '*', command: 'printf unapproved'}", 1)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), changed)
	before := a4State(t, home, config)
	result, err = service.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || len(result.NeedsTrust) != 1 || !result.Changes[0].NativeChanged {
		t.Fatalf("%+v %v", result, err)
	}
	a4AssertState(t, before, home, config)
}

func TestNativeOnlyPackageLifecycle(t *testing.T) {
	requirePOSIXFixture(t, "native lifecycle uses POSIX commands")
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			service, home, config, source, _ := nativeInstallFixture(t, route)
			header, rest, _ := strings.Cut(nativePackageYAML("native-test"), "rules:\n")
			_, hooks, _ := strings.Cut(rest, "native_hooks:\n")
			manifest := header + "native_hooks:\n" + hooks
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
			provider := nativeProvider(route)
			planned, err := service.Plan(context.Background(), PlanRequest{Source: source, Harnesses: []string{provider}})
			if err != nil || len(planned.Reports) != 1 {
				t.Fatalf("%+v %v", planned, err)
			}
			report := planned.Reports[0]
			if len(report.Plan.Rules) != 0 || len(report.Plan.NativeHooks) != 2 || len(report.Tokens.Rules) != 0 || len(report.Tokens.Unestimated) != 2 || report.Tokens.AlwaysOn.Tokens != 0 {
				t.Fatalf("hook-only plan: %+v", report)
			}
			request := InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{provider}, Routes: map[string]string{provider: route}}
			if _, err := service.Install(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			assertNativeInstalled(t, home, config, route, "native-test")
			index := a6Index(t, home)
			if len(index.Harnesses[provider]) != 0 || len(index.Static[provider]) != 0 {
				t.Fatal("hook-only package emitted runtime rules")
			}
			if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed {
				t.Fatalf("%+v %v", result, err)
			}
			if result, err := service.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil || result.Changed {
				t.Fatalf("%+v %v", result, err)
			}
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.Replace(manifest, "version: 0.1.0", "version: 0.1.1", 1))
			if result, err := service.Update(context.Background(), UpdateRequest{Home: home, Name: "native-test", Yes: true}); err != nil || !result.Changed {
				t.Fatalf("%+v %v", result, err)
			}
			if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeFingerprintExcludesNotesAndDisabledHooks(t *testing.T) {
	item := lockPackage{Source: lockSource{Kind: "dir", Path: "fixture"}}
	original := nativePackageYAML("fingerprint")
	fingerprint := func(manifest string) string {
		t.Helper()
		value, err := nativeSourceFingerprint(item, map[string][]byte{"STYLE.yaml": []byte(manifest)})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	baseline := fingerprint(original)
	for _, change := range []string{
		strings.Replace(original, "  - id: later\n", "  - id: later\n    description: Notes.\n", 1),
		strings.Replace(original, "native_hooks:\n", "native_hooks:\n  - id: disabled\n    enabled: false\n    codex: {event: Stop, command: unused}\n", 1),
	} {
		if fingerprint(change) != baseline {
			t.Fatal("non-executable edit changed execution fingerprint")
		}
	}
	for _, change := range []string{
		strings.Replace(original, "priority: 80", "priority: 10", 1),
		strings.Replace(original, "timeout: 19", "timeout: 20", 1),
		strings.Replace(original, "claude-code: {event: PreToolUse, matcher: '*', command: 'printf earlier'}", "claude-code: {event: Stop, command: notify}", 1),
	} {
		if fingerprint(change) == baseline {
			t.Fatal("execution edit did not change fingerprint")
		}
	}
}
