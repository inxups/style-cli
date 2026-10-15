package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestTrustDoesNotApproveAnotherOrdinaryPackage(t *testing.T) {
	service, home, source, _ := syncFixture(t, false)
	second := writeInstallPackage(t, filepath.Join(t.TempDir(), "second"), strings.ReplaceAll(syncManifest, "editable", "second"))
	writeSyncFile(t, filepath.Join(second, "rules", "shell.md"), syncRule)
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: second, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{source, filepath.Join(home, "packages", "second")} {
		writeSyncFile(t, filepath.Join(dir, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original", "Changed"))
	}
	beforeLock, _ := loadLock(home)
	called := false
	result, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Confirm: func(SyncResult) (bool, error) { called = true; return true, nil }})
	if err != nil || !called || !result.Changed || !reflect.DeepEqual(result.NeedsTrust, []string{"second"}) {
		t.Fatalf("trust = %+v, %v; confirmation=%t", result, err, called)
	}
	afterLock, _ := loadLock(home)
	if !reflectStringMap(beforeLock.Packages[1].Files, afterLock.Packages[1].Files) {
		t.Fatal("trust approved unrelated bytes")
	}
	index := string(readSyncFile(t, filepath.Join(home, "build", "index.json")))
	if !strings.Contains(index, "Changed dynamic instruction.") || !strings.Contains(index, "Original dynamic instruction.") {
		t.Fatal("rebuild did not retain the other package's approved body")
	}
	secondResult, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "second", Yes: true})
	if err != nil || !secondResult.Changed {
		t.Fatalf("second approval = %+v, %v", secondResult, err)
	}
	if result, err := service.Sync(context.Background(), SyncRequest{Home: home}); err != nil || result.Changed || len(result.NeedsTrust) != 0 {
		t.Fatalf("sync after both approvals = %+v, %v", result, err)
	}
}

func TestTrustRevalidatesChangedPackageBeforeMutation(t *testing.T) {
	for _, tc := range []struct{ name, manifest, want string }{
		{"identity", strings.ReplaceAll(syncManifest, "name: editable", "name: renamed"), "cannot change its name"},
		{"schema", strings.ReplaceAll(syncManifest, "target: system", "target: missing"), "validation"},
		{"runtime", syncManifest + "requires:\n  runtime: '>=99.0.0'\n", "requires.runtime"},
		{"missing-setting", strings.ReplaceAll(syncManifest, "default: direct", "description: value") + "settings-extra: invalid\n", "validation"},
		{"replace", strings.ReplaceAll(syncManifest, "target: system", "target: system\n    mode: replace"), "allow-replace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, home, source, config := syncFixture(t, false)
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), tc.manifest)
			before := installedTree(t, home, config)
			_, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
			if !reflect.DeepEqual(before, installedTree(t, home, config)) {
				t.Fatal("rejected trust mutated installation")
			}
		})
	}
	t.Run("required-new-setting", func(t *testing.T) {
		service, home, source, config := syncFixture(t, false)
		manifest := strings.ReplaceAll(syncManifest, "name: voice\n    default: direct", "name: unknown\n    description: new setting")
		manifest = strings.ReplaceAll(manifest, "settings.voice", "settings.unknown")
		writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), manifest)
		before := installedTree(t, home, config)
		if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true}); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("missing new setting error = %v", err)
		}
		if !reflect.DeepEqual(before, installedTree(t, home, config)) {
			t.Fatal("missing setting mutated installation")
		}
	})
}

func TestTrustReplaceRequiresExplicitFlagAndRetainsUniqueness(t *testing.T) {
	service, home, source, _ := syncFixture(t, false)
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(syncManifest, "target: system", "target: system\n    mode: replace"))
	if result, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true, AllowReplace: true}); err != nil || !result.Changed {
		t.Fatalf("replace approval = %+v, %v", result, err)
	}
	second := writeInstallPackage(t, filepath.Join(t.TempDir(), "second"), "spec: style/v0.3\nname: second\nversion: 0.1.0\ndescription: second\nrules:\n  - id: base\n    target: system\n    text: second\n")
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: second, Yes: true}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "packages", "second", "STYLE.yaml")
	writeSyncFile(t, path, strings.ReplaceAll(string(readSyncFile(t, path)), "target: system", "target: system\n    mode: replace"))
	before := installedTree(t, home)
	if _, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "second", Yes: true, AllowReplace: true}); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("replace uniqueness error = %v", err)
	}
	if !reflect.DeepEqual(before, installedTree(t, home)) {
		t.Fatal("replace conflict mutated installation")
	}
}

func TestTrustT003AcrossPackagesBlocksWithoutWriting(t *testing.T) {
	service, home, source, config := syncFixture(t, false)
	second := writeInstallPackage(t, filepath.Join(t.TempDir(), "second"), "spec: style/v0.3\nname: second\nversion: 0.1.0\ndescription: second\nrules:\n  - id: guard\n    target: tool\n    tool: shell\n    at: before\n    text: '"+strings.Repeat("Second guard. ", 8)+"'\n")
	if _, err := service.Install(context.Background(), InstallRequest{Home: home, Source: second, Yes: true}); err != nil {
		t.Fatal(err)
	}
	definition := service.definitions[adapter.ProviderID("codex")]
	definition.Limits.Input = tokens.InputLimit{Value: 400, Unit: tokens.CharsUnit}
	service.definitions[adapter.ProviderID("codex")] = definition
	writeSyncFile(t, filepath.Join(source, "rules", "shell.md"), strings.ReplaceAll(syncRule, "Original dynamic instruction.", strings.Repeat("Expanded instruction. ", 10)))
	before := installedTree(t, home, config)
	_, err := service.Trust(context.Background(), TrustRequest{Home: home, Name: "editable", Yes: true})
	var blocked *BlockedError
	if !errors.As(err, &blocked) || !strings.Contains(err.Error(), "installed estimate") {
		t.Fatalf("combined T003 error = %v", err)
	}
	if !reflect.DeepEqual(before, installedTree(t, home, config)) {
		t.Fatal("T003 rejection wrote files")
	}
}

func TestTrustRejectsStaleTransactionAndApprovalTampering(t *testing.T) {
	for _, kind := range []string{"stale-lock", "stale-route", "baseline", "missing-baseline", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			service, home, source, config := syncFixture(t, false)
			writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(syncManifest, "original static", "updated static"))
			before := installedTree(t, home, config)
			request := TrustRequest{Home: home, Name: "editable", Yes: true}
			if kind == "baseline" {
				writeSyncFile(t, approvalPath(home, "editable"), "tampered baseline")
			} else if kind == "missing-baseline" {
				if err := os.Remove(approvalPath(home, "editable")); err != nil {
					t.Fatal(err)
				}
			} else {
				request.Yes = false
				request.Confirm = func(SyncResult) (bool, error) {
					switch kind {
					case "stale-lock":
						writeSyncFile(t, filepath.Join(home, "style.lock.yaml"), "external lock edit")
					case "stale-route":
						writeSyncFile(t, filepath.Join(config, "config.toml"), "external route edit")
					case "cancel":
						return false, context.Canceled
					}
					return true, nil
				}
			}
			_, err := service.Trust(context.Background(), request)
			if err == nil {
				t.Fatal("trust succeeded with invalid/stale approval")
			}
			for path, want := range before {
				if kind == "stale-lock" && path == filepath.Join(home, "style.lock.yaml") || kind == "stale-route" && path == filepath.Join(config, "config.toml") || (kind == "baseline" || kind == "missing-baseline") && path == approvalPath(home, "editable") {
					continue
				}
				if got := string(readSyncFile(t, path)); got != want {
					t.Fatalf("failure changed %s", path)
				}
			}
		})
	}
}
