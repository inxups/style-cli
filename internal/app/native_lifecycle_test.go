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
	"testing"
)

func nativeProvider(route string) string {
	if route == "claude-user" {
		return "claude-code"
	}
	return "codex"
}

func nativeHookPath(home, config, route string) string {
	if route == "codex-plugin" {
		return filepath.Join(home, "adapters", "codex", "plugin", "hooks", "hooks.json")
	}
	if route == "claude-user" {
		return filepath.Join(config, "settings.json")
	}
	return filepath.Join(config, "hooks.json")
}

func TestA4NativeInstallDisclosesBeforeConfirmationWithoutWriting(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	for _, route := range []string{"codex-plugin", "codex-user", "claude-user"} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%t", route, jsonOutput), func(t *testing.T) {
				service, home, config, source, _ := nativeInstallFixture(t, route)
				t.Setenv("STYLE_HOME", home)
				before := installedTree(t, config)
				args := []string{"install", source, "--harness=" + nativeProvider(route), "--route=" + nativeProvider(route) + "=" + route}
				if jsonOutput {
					args = append(args, "--json")
				}
				var out, errOut bytes.Buffer
				if code := RunWithService(context.Background(), service, filepath.Dir(source), args, &out, &errOut); code != ExitInvalid {
					t.Fatalf("unconfirmed install exit=%d output=%s stderr=%s", code, out.String(), errOut.String())
				}
				if _, err := os.Stat(home); !os.IsNotExist(err) {
					t.Fatalf("preview created STYLE_HOME: %v", err)
				}
				if !reflect.DeepEqual(before, installedTree(t, config)) {
					t.Fatal("preview changed harness files")
				}
				for _, required := range []string{nativeProvider(route), "native-test/earlier", "PreToolUse", "printf earlier", route} {
					if !strings.Contains(out.String(), required) {
						t.Fatalf("missing %q: %s", required, out.String())
					}
				}
				if strings.Contains(out.String(), ": --owner=style") {
					t.Fatal("disclosure contains ownership prefix")
				}
				if jsonOutput {
					var preview InstallResult
					if err := json.Unmarshal(out.Bytes(), &preview); err != nil || !preview.AwaitingConfirmation || len(preview.Native) != 2 {
						t.Fatalf("preview=%+v err=%v", preview, err)
					}
					wantTrust := map[string]string{"codex-plugin": "user-confirmation", "codex-user": "preseed", "claude-user": "workspace"}[route]
					if string(preview.Native[0].Trust) != wantTrust || preview.Native[0].TrustSummary == "" || preview.Native[0].Timeout != 60 {
						t.Fatalf("native disclosure=%+v", preview.Native)
					}
				}
				out.Reset()
				errOut.Reset()
				if code := RunWithService(context.Background(), service, filepath.Dir(source), append(args, "--yes"), &out, &errOut); code != ExitOK {
					t.Fatalf("confirmed exit=%d %s", code, errOut.String())
				}
				assertNativeInstalled(t, home, config, route, "native-test")
				listed, err := service.List(context.Background(), ListRequest{Home: home})
				if err != nil || listed.Packages[0].NativeCount != 2 {
					t.Fatalf("native count=%+v err=%v", listed, err)
				}
			})
		}
	}
}

func TestA4NativeSourceChangesRequireTrustForOrdinaryAndLinkedPackages(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	mutations := []struct {
		name          string
		change        func(string) string
		effectiveDiff bool
	}{
		{"command", func(s string) string { return strings.ReplaceAll(s, "printf earlier", "printf approved") }, true},
		{"event", func(s string) string { return strings.ReplaceAll(s, "event: PreToolUse", "event: PostToolUse") }, true},
		{"matcher", func(s string) string {
			return strings.ReplaceAll(strings.ReplaceAll(s, "matcher: '^Bash$'", "matcher: '^Edit$'"), "matcher: '*'", "matcher: 'Edit'")
		}, true},
		{"timeout", func(s string) string { return strings.ReplaceAll(s, "timeout: 19", "timeout: 23") }, true},
		{"add", func(s string) string {
			return s + "  - id: added\n    target: native\n    codex: {event: Stop, command: 'printf added'}\n    claude-code: {event: Stop, command: 'printf added'}\n"
		}, true},
		{"remove", func(s string) string { before, _, _ := strings.Cut(s, "  - id: earlier\n"); return before }, true},
		{"priority", func(s string) string { return strings.ReplaceAll(s, "priority: 80", "priority: 5") }, false},
		{"explicit-default-timeout", func(s string) string {
			return strings.ReplaceAll(s, "command: 'printf earlier'}", "command: 'printf earlier', timeout: 60}")
		}, true},
	}
	for _, route := range []string{"codex-plugin", "codex-user", "claude-user"} {
		for _, link := range []bool{false, true} {
			for _, mutation := range mutations {
				t.Run(fmt.Sprintf("%s/link=%t/%s", route, link, mutation.name), func(t *testing.T) {
					service, home, config, source, _ := nativeInstallFixture(t, route)
					t.Setenv("STYLE_HOME", home)
					provider := nativeProvider(route)
					if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Yes: true, Link: link, Harnesses: []string{provider}, Routes: map[string]string{provider: route}}); err != nil {
						t.Fatal(err)
					}
					if link {
						if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
							t.Fatal(err)
						}
					} else {
						source = filepath.Join(home, "packages", "native-test")
					}
					writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), mutation.change(nativePackageYAML("native-test")))
					before := installedTree(t, home, config)
					var out, errOut bytes.Buffer
					if code := RunWithService(context.Background(), service, source, []string{"sync", "--json"}, &out, &errOut); code != ExitSyncChanged {
						t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
					}
					if strings.Contains(out.String(), "printf") {
						t.Fatal("sync summary exposed native commands")
					}
					preview, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test"})
					if err != nil || !preview.AwaitingConfirmation || len(preview.Changes) != 1 || !preview.Changes[0].NativeChanged {
						t.Fatalf("preview=%+v err=%v", preview, err)
					}
					if (len(preview.Changes[0].NativeDiff) > 0) != mutation.effectiveDiff {
						t.Fatalf("native diff=%+v", preview.Changes[0].NativeDiff)
					}
					if !reflect.DeepEqual(before, installedTree(t, home, config)) {
						t.Fatal("blocked sync or trust preview wrote installation")
					}
					out.Reset()
					errOut.Reset()
					if code := RunWithService(context.Background(), service, source, []string{"trust", "native-test"}, &out, &errOut); code != ExitInvalid || !strings.Contains(out.String(), "Native entry diff:") {
						t.Fatalf("trust preview exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
					}
					if !reflect.DeepEqual(before, installedTree(t, home, config)) {
						t.Fatal("CLI trust preview wrote installation")
					}
					if result, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true}); err != nil || !result.Changed {
						t.Fatalf("trust=%+v err=%v", result, err)
					}
					if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed || len(result.NeedsTrust) > 0 {
						t.Fatalf("sync after trust=%+v err=%v", result, err)
					}
					checks, err := service.checkNative(context.Background(), home, mustLoadNativeLock(t, home), "")
					if err != nil {
						t.Fatal(err)
					}
					for _, check := range checks {
						if check.Status != "ok" || check.TrustStatus != "" && check.TrustStatus != "match" {
							t.Fatalf("approved configuration=%+v", check)
						}
					}
				})
			}
		}
	}
}

func mustLoadNativeLock(t *testing.T, home string) lockDocument {
	t.Helper()
	lock, err := loadLock(home)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func TestA4DoctorNativeStatesAndConfirmedRepairs(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	for _, route := range []string{"codex-plugin", "codex-user", "claude-user"} {
		for _, scenario := range []string{"ok", "missing", "command", "matcher", "timeout", "event", "duplicate", "trust"} {
			if scenario == "trust" && route != "codex-user" {
				continue
			}
			t.Run(route+"/"+scenario, func(t *testing.T) {
				service, home, config, source, _ := nativeInstallFixture(t, route)
				provider := nativeProvider(route)
				if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Yes: true, Harnesses: []string{provider}, Routes: map[string]string{provider: route}}); err != nil {
					t.Fatal(err)
				}
				path := nativeHookPath(home, config, route)
				var document struct {
					External string                       `json:"external,omitempty"`
					Hooks    map[string][]nativeTestGroup `json:"hooks"`
					Restore  json.RawMessage              `json:"_agenthooks_restore,omitempty"`
				}
				data := readSyncFile(t, path)
				// Preserve the complete top-level object, including restoration metadata.
				var top map[string]json.RawMessage
				if err := json.Unmarshal(data, &top); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &document); err != nil {
					t.Fatal(err)
				}
				index := -1
				for i, group := range document.Hooks["PreToolUse"] {
					if strings.Contains(group.Hooks[0].Command, "--native=native-test/earlier;") {
						index = i
					}
				}
				if index < 0 {
					t.Fatal("native fixture missing")
				}
				group := document.Hooks["PreToolUse"][index]
				switch scenario {
				case "missing":
					document.Hooks["PreToolUse"] = append(document.Hooks["PreToolUse"][:index], document.Hooks["PreToolUse"][index+1:]...)
				case "command":
					document.Hooks["PreToolUse"][index].Hooks[0].Command += " --tampered"
				case "matcher":
					document.Hooks["PreToolUse"][index].Matcher = "Edited"
				case "timeout":
					document.Hooks["PreToolUse"][index].Hooks[0].Timeout = 29
				case "event":
					document.Hooks["PreToolUse"] = append(document.Hooks["PreToolUse"][:index], document.Hooks["PreToolUse"][index+1:]...)
					document.Hooks["PostToolUse"] = append(document.Hooks["PostToolUse"], group)
				case "duplicate":
					document.Hooks["PreToolUse"] = append(document.Hooks["PreToolUse"], group)
				case "trust":
					trust := readSyncFile(t, filepath.Join(config, "config.toml"))
					writeSyncFile(t, filepath.Join(config, "config.toml"), strings.ReplaceAll(string(trust), "trusted_hash = \"sha256:", "trusted_hash = \"tampered:"))
				}
				if scenario != "ok" && scenario != "trust" {
					top["hooks"], _ = json.Marshal(document.Hooks)
					data, _ = json.Marshal(top)
					if err := os.WriteFile(path, data, 0o640); err != nil {
						t.Fatal(err)
					}
				}
				before := installedTree(t, home, config)
				result, err := service.Doctor(context.Background(), DoctorRequest{Home: home, Shell: "/bin/sh"})
				if err != nil || len(result.Native) != 2 {
					t.Fatalf("doctor native=%+v err=%v diagnostics=%+v", result.Native, err, result.Diagnostics)
				}
				want := "modified"
				if scenario == "ok" || scenario == "trust" {
					want = "ok"
				}
				if scenario == "missing" {
					want = "missing"
				}
				var actual NativeHookCheck
				for _, check := range result.Native {
					if check.Rule == "native-test/earlier" {
						actual = check
					}
				}
				if actual.Status != want {
					t.Fatalf("native status=%+v want=%s", actual, want)
				}
				if scenario != "ok" && scenario != "trust" && !doctorHas(result, "native-"+want) {
					t.Fatalf("missing warning: %+v", result.Diagnostics)
				}
				if scenario == "trust" && (actual.TrustStatus != "mismatch" || !doctorHas(result, "native-trust")) {
					t.Fatalf("trust check=%+v diagnostics=%+v", actual, result.Diagnostics)
				}
				if !reflect.DeepEqual(before, installedTree(t, home, config)) {
					t.Fatal("doctor modified native configuration")
				}
				if scenario != "ok" {
					if sync, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(sync.NeedsTrust) != 1 {
						t.Fatalf("repair sync=%+v err=%v", sync, err)
					}
					if !reflect.DeepEqual(before, installedTree(t, home, config)) {
						t.Fatal("unconfirmed repair wrote configuration")
					}
					if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
						t.Fatal(err)
					}
					checks, err := service.checkNative(context.Background(), home, mustLoadNativeLock(t, home), "")
					if err != nil {
						t.Fatal(err)
					}
					for _, check := range checks {
						if check.Status != "ok" || check.TrustStatus != "" && check.TrustStatus != "match" {
							t.Fatalf("repair failed: %+v", check)
						}
					}
				}
			})
		}
	}
}

func TestA4NativeConfirmationUsesCapturedBytesAndDetachedReports(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	service, home, config, source, _ := nativeInstallFixture(t, "codex-user")
	original := nativePackageYAML("native-test")
	approved := strings.ReplaceAll(original, "printf earlier", "printf approved")
	unreviewed := strings.ReplaceAll(original, "printf earlier", "printf unreviewed")
	result, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Link: true,
		Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}, Confirm: func(preview InstallResult) (bool, error) {
			if len(preview.Native) != 2 || preview.Native[0].Command != "printf earlier" {
				t.Fatalf("install preview=%+v", preview.Native)
			}
			if _, err := os.Stat(home); !os.IsNotExist(err) {
				t.Fatal("install wrote before confirmation")
			}
			preview.Native[0].Command = "preview mutation"
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), approved)
			return true, nil
		}})
	if err != nil || result.Native[0].Command != "printf earlier" {
		t.Fatalf("install=%+v err=%v", result, err)
	}
	data := readSyncFile(t, filepath.Join(config, "hooks.json"))
	if bytes.Contains(data, []byte("printf approved")) || !bytes.Contains(data, []byte("printf earlier")) {
		t.Fatal("install reread source during confirmation")
	}
	// Exercise the reconstruction path for approval snapshots created by T12.
	lock := mustLoadNativeLock(t, home)
	baseline, _, err := loadApproval(home, lock.Packages[0])
	if err != nil {
		t.Fatal(err)
	}
	baseline.Native = nil
	encoded, _ := json.Marshal(baseline)
	writeSyncFile(t, approvalPath(home, "native-test"), string(encoded))
	lock.Packages[0].Approval = hashBytes(encoded)
	if err := writeLock(home, lock); err != nil {
		t.Fatal(err)
	}
	trusted, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Confirm: func(preview SyncResult) (bool, error) {
		if len(preview.Changes[0].NativeDiff) != 1 || preview.Changes[0].NativeDiff[0].Before.Command != "printf earlier" || preview.Changes[0].NativeDiff[0].After.Command != "printf approved" {
			t.Fatalf("legacy native diff=%+v", preview.Changes)
		}
		preview.Native[0].Command = "callback mutation"
		return true, nil
	}})
	if err != nil || !trusted.Changed {
		t.Fatalf("trust=%+v err=%v", trusted, err)
	}
	data = readSyncFile(t, filepath.Join(config, "hooks.json"))
	if bytes.Contains(data, []byte("printf unreviewed")) || bytes.Contains(data, []byte("callback mutation")) || !bytes.Contains(data, []byte("printf approved")) {
		t.Fatal("trust did not install the confirmed native bytes")
	}
	lock = mustLoadNativeLock(t, home)
	if lock.Packages[0].Files["STYLE.yaml"] != hashBytes([]byte(approved)) {
		t.Fatal("lock did not retain confirmed source hash")
	}
	// Edits during confirmation are rejected by the D2 commit guard; edits
	// after a successful approval still require a separate native review.
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), unreviewed)
	if sync, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(sync.NeedsTrust) != 1 {
		t.Fatalf("unreviewed native edit escaped: %+v %v", sync, err)
	}
}

func TestA4TrustKeepsOtherLinkedNativeChangesUnapproved(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	service, home, config, first, _ := nativeInstallFixture(t, "codex-user")
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(first), "second"), nativePackageYAML("second"))
	for _, source := range []string{first, second} {
		if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Yes: true, Link: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"native-test", "second"} {
		if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: name, Yes: true}); err != nil {
			t.Fatal(err)
		}
	}
	for i, source := range []string{first, second} {
		name := []string{"native-test", "second"}[i]
		writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(nativePackageYAML(name), "printf earlier", "printf "+name+"-changed"))
	}
	oldLock := mustLoadNativeLock(t, home)
	trusted, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true})
	if err != nil || !reflect.DeepEqual(trusted.NeedsTrust, []string{"second"}) {
		t.Fatalf("trust=%+v err=%v", trusted, err)
	}
	current := mustLoadNativeLock(t, home)
	if !reflectStringMap(oldLock.Packages[1].Files, current.Packages[1].Files) {
		t.Fatal("trust approved another link's native source")
	}
	data := readSyncFile(t, filepath.Join(config, "hooks.json"))
	if !bytes.Contains(data, []byte("printf native-test-changed")) || bytes.Contains(data, []byte("printf second-changed")) {
		t.Fatalf("other native command changed: %s", data)
	}
	if sync, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || !reflect.DeepEqual(sync.NeedsTrust, []string{"second"}) {
		t.Fatalf("pending native change lost: %+v %v", sync, err)
	}
}

func TestA4LinkedNativePackageStillAutoSyncsOrdinaryEdits(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	service, home, config, source, _ := nativeInstallFixture(t, "codex-user")
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Yes: true, Link: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(nativePackageYAML("native-test"), "Be careful.", "Updated ordinary instruction."))
	result, err := service.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || !result.Changed || len(result.NeedsTrust) != 0 {
		t.Fatalf("ordinary edit=%+v err=%v", result, err)
	}
	assertNativeInstalled(t, home, config, "codex-user", "native-test")
	if !bytes.Contains(readSyncFile(t, filepath.Join(home, "build", "index.json")), []byte("Updated ordinary instruction.")) {
		t.Fatal("ordinary link edit did not compile")
	}
}

func TestA4ClampedNativeTimeoutChangeStillRequiresTrust(t *testing.T) {
	requirePOSIXFixture(t, "author native lifecycle requires POSIX")
	service, home, _, source, _ := nativeInstallFixture(t, "codex-user")
	manifest := "spec: style/v0.3\nname: native-test\nversion: 0.1.0\ndescription: cap\nrules:\n  - id: end\n    target: native\n    codex: {event: SessionEnd, command: 'true', timeout: 10}\n"
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: source, Yes: true, Link: true, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": "codex-user"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test", Yes: true}); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(manifest, "timeout: 10", "timeout: 20"))
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || len(result.NeedsTrust) != 1 {
		t.Fatalf("clamped timeout bypassed trust: %+v %v", result, err)
	}
	preview, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "native-test"})
	if err != nil || !preview.AwaitingConfirmation || !preview.Changes[0].NativeChanged || len(preview.Changes[0].NativeDiff) != 1 || preview.Native[0].Timeout != 3 {
		t.Fatalf("clamped timeout preview=%+v err=%v", preview, err)
	}
	difference := preview.Changes[0].NativeDiff[0]
	if difference.Before.DeclaredTimeout != 10 || difference.After.DeclaredTimeout != 20 || difference.Before.Timeout != 3 || difference.After.Timeout != 3 {
		t.Fatalf("declared timeout diff=%+v", difference)
	}
}
