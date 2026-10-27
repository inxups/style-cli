package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func registerUninstallPlugin(t *testing.T, config, plugin string) string {
	t.Helper()
	path := filepath.Join(config, "config.toml")
	data := readSyncFile(t, path)
	data = append(data, []byte("\n[marketplaces.style]\nsource_type = 'local'\nsource = "+strconv.Quote(filepath.Dir(plugin))+"\n\n[plugins.\"style-runtime@style\"]\nenabled = true\n")...)
	writeSyncFile(t, path, string(data))
	cache := filepath.Dir(codexPluginCache(config))
	for _, version := range []string{"0.1.0", "old-version"} {
		writeSyncFile(t, filepath.Join(cache, version, "hooks/hooks.json"), "cached hooks")
		writeSyncFile(t, filepath.Join(cache, version, "bin/style"), "cached launcher")
	}
	return cache
}

func TestUninstallLastPackageCleansCodexPlugin(t *testing.T) {
	requireFixtureRoute(t, "codex-plugin")
	for _, all := range []bool{false, true} {
		t.Run(strconv.FormatBool(all), func(t *testing.T) {
			root := t.TempDir()
			home, config, plugin := filepath.Join(root, "style"), filepath.Join(root, "codex"), filepath.Join(root, "plugin")
			service := testCodexInstallService(t, config, plugin)
			userConfig := "# user config\nmodel = 'keep'\n\n[plugins.\"other@elsewhere\"]\nenabled = true\n"
			writeSyncFile(t, filepath.Join(config, "config.toml"), userConfig)
			source := writeInstallPackage(t, filepath.Join(root, "source"), a4Manifest("first", "First."))
			if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Harnesses: []string{"codex"}, Yes: true}); err != nil {
				t.Fatal(err)
			}
			cache := registerUninstallPlugin(t, config, plugin)
			otherCache := filepath.Join(config, "plugins/cache/style/other/0.1.0/hooks.json")
			writeSyncFile(t, otherCache, "other plugin")
			before := a4State(t, home, config)
			request := UninstallRequest{Home: home, All: all}
			if !all {
				request.Name = "first"
			}
			if result, err := service.Uninstall(context.Background(), request); err == nil || !result.AwaitingConfirmation {
				t.Fatalf("unconfirmed uninstall: %+v %v", result, err)
			}
			a4AssertState(t, before, home, config)
			request.Yes = true
			if result, err := service.Uninstall(context.Background(), request); err != nil || !result.Changed || len(result.Remaining) != 0 {
				t.Fatalf("uninstall: %+v %v", result, err)
			}
			for _, path := range []string{cache, filepath.Join(home, "style.lock.yaml"), filepath.Join(plugin, ".codex-plugin/plugin.json")} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("uninstall left %s: %v", path, err)
				}
			}
			got := readSyncFile(t, filepath.Join(config, "config.toml"))
			if !bytes.HasPrefix(got, []byte(userConfig)) || bytes.Contains(got, []byte("style-runtime@style")) || bytes.Contains(got, []byte("marketplaces.style")) {
				t.Fatalf("remaining config: %s", got)
			}
			if string(readSyncFile(t, otherCache)) != "other plugin" {
				t.Fatal("other cache changed")
			}
		})
	}
}

func TestUninstallKeepsCodexPluginWhilePackagesRemain(t *testing.T) {
	requireFixtureRoute(t, "codex-plugin")
	root := t.TempDir()
	home, config, plugin := filepath.Join(root, "style"), filepath.Join(root, "codex"), filepath.Join(root, "plugin")
	service := testCodexInstallService(t, config, plugin)
	for _, name := range []string{"first", "second"} {
		source := writeInstallPackage(t, filepath.Join(root, name), a4Manifest(name, name))
		if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Harnesses: []string{"codex"}, Yes: true}); err != nil {
			t.Fatal(err)
		}
	}
	cache := registerUninstallPlugin(t, config, plugin)
	cacheBefore := a4State(t, cache, cache)
	if result, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "first", Yes: true}); err != nil || len(result.Remaining) != 1 || result.Remaining[0] != "second" {
		t.Fatalf("partial uninstall: %+v %v", result, err)
	}
	a4AssertState(t, cacheBefore, cache, cache)
	got := readSyncFile(t, filepath.Join(config, "config.toml"))
	if !bytes.Contains(got, []byte("style-runtime@style")) || !bytes.Contains(got, []byte("marketplaces.style")) {
		t.Fatalf("plugin registration removed: %s", got)
	}
	readSyncFile(t, filepath.Join(plugin, ".codex-plugin/plugin.json"))
}

func TestUninstallAllCleansResidualCodexPlugin(t *testing.T) {
	requireFixtureRoute(t, "codex-plugin")
	for _, mode := range []string{"no-lock", "empty-lock", "cache-only"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			home, config, plugin := filepath.Join(root, "style"), filepath.Join(root, "codex"), filepath.Join(root, "plugin")
			t.Setenv("STYLE_HOME", home)
			service := testCodexInstallService(t, config, plugin)
			writeSyncFile(t, filepath.Join(config, "config.toml"), "# keep user config\n")
			cache := registerUninstallPlugin(t, config, plugin)
			if mode == "empty-lock" {
				writeSyncFile(t, filepath.Join(home, "style.lock.yaml"), "format: 1\npackages: []\n")
			}
			if mode == "cache-only" {
				if err := os.Remove(filepath.Join(config, "config.toml")); err != nil {
					t.Fatal(err)
				}
			}
			before := a4State(t, home, config)
			var out, errOut bytes.Buffer
			args := []string{"uninstall", "--all", "--json"}
			if code := RunWithServiceInput(context.Background(), service, root, args, strings.NewReader("n\n"), &out, &errOut); code != ExitInvalid {
				t.Fatalf("declined cleanup: %d %s %s", code, out.String(), errOut.String())
			}
			a4AssertState(t, before, home, config)
			out.Reset()
			errOut.Reset()
			if code := RunWithServiceInput(context.Background(), service, root, args, approve(), &out, &errOut); code != ExitOK {
				t.Fatalf("cleanup: %d %s %s", code, out.String(), errOut.String())
			}
			var result UninstallResult
			if err := decodeReport(out.Bytes(), &result); err != nil || !result.Changed || len(result.Removed) != 0 {
				t.Fatalf("cleanup report: %+v %v", result, err)
			}
			if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("cache remains: %v", err)
			}
			if mode != "cache-only" && string(readSyncFile(t, filepath.Join(config, "config.toml"))) != "# keep user config\n\n" {
				t.Fatal("user config changed")
			}
			if result, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil || result.Changed {
				t.Fatalf("repeated cleanup: %+v %v", result, err)
			}
		})
	}
}

func TestUninstallPluginCleanupRejectsChangedInputs(t *testing.T) {
	requireFixtureRoute(t, "codex-plugin")
	for _, mode := range []string{"cache", "registration", "foreign-marketplace"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			home, config, plugin := filepath.Join(root, "style"), filepath.Join(root, "codex"), filepath.Join(root, "plugin")
			service := testCodexInstallService(t, config, plugin)
			source := writeInstallPackage(t, filepath.Join(root, "source"), a4Manifest("first", "First."))
			if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Harnesses: []string{"codex"}, Yes: true}); err != nil {
				t.Fatal(err)
			}
			cache := registerUninstallPlugin(t, config, plugin)
			configPath := filepath.Join(config, "config.toml")
			if mode == "foreign-marketplace" {
				data := readSyncFile(t, configPath)
				writeSyncFile(t, configPath, strings.Replace(string(data), strconv.Quote(filepath.Dir(plugin)), "'/different/style'", 1))
				before := a4State(t, home, config)
				if result, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err == nil || result.Changed || !strings.Contains(err.Error(), "does not belong") {
					t.Fatalf("foreign cleanup: %+v %v", result, err)
				}
				a4AssertState(t, before, home, config)
				return
			}
			var afterConfirmation map[string]a4FileState
			result, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Confirm: func(UninstallResult) (bool, error) {
				path := configPath
				if mode == "cache" {
					path = filepath.Join(cache, "0.1.0/hooks/hooks.json")
				}
				writeSyncFile(t, path, string(readSyncFile(t, path))+"\n# updated during confirmation\n")
				afterConfirmation = a4State(t, home, config)
				return true, nil
			}})
			if !errors.Is(err, errManagementChanged) || result.Changed {
				t.Fatalf("changed inputs accepted: %+v %v", result, err)
			}
			a4AssertState(t, afterConfirmation, home, config)
		})
	}
}

func TestUninstallPluginCleanupCancellationRestoresStorage(t *testing.T) {
	requireFixtureRoute(t, "codex-plugin")
	setup := func(t *testing.T) (*Service, string, string) {
		t.Helper()
		root := t.TempDir()
		home, config, plugin := filepath.Join(root, "style"), filepath.Join(root, "codex"), filepath.Join(root, "plugin")
		service := testCodexInstallService(t, config, plugin)
		source := writeInstallPackage(t, filepath.Join(root, "source"), a4Manifest("first", "First."))
		if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Harnesses: []string{"codex"}, Yes: true}); err != nil {
			t.Fatal(err)
		}
		registerUninstallPlugin(t, config, plugin)
		return service, home, config
	}
	service, home, _ := setup(t)
	count := &a4CancelAt{Context: context.Background(), at: 1000000}
	if _, err := service.Uninstall(count, UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for at := 1; at <= count.checks; at++ {
		t.Run(strconv.Itoa(at), func(t *testing.T) {
			service, home, config := setup(t)
			before := a4State(t, home, config)
			fault := &a4CancelAt{Context: context.Background(), at: at}
			result, err := service.Uninstall(fault, UninstallRequest{Home: home, All: true, Yes: true})
			if !errors.Is(err, context.Canceled) || result.Changed || !fault.reached {
				t.Fatalf("cancellation: %+v %v fault=%+v", result, err, fault)
			}
			a4AssertState(t, before, home, config)
		})
	}
}

func TestUninstallRemovesOnlyStyleOwnerAndRestoresLastRoute(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	t.Setenv("STYLE_HOME", styleHome)
	configDir := filepath.Join(root, "codex")
	pluginDir := filepath.Join(styleHome, "adapters", "codex", "plugin")
	service := testCodexInstallService(t, configDir, pluginDir)
	configPath := filepath.Join(configDir, "config.toml")
	beforeExternal := []byte("# external\n")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, beforeExternal, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), InstallRequest{Source: filepath.Join(repositoryRoot(t), "testdata", "packages", "terse-reviewer"), Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	var output, errOutput bytes.Buffer
	code := RunWithServiceInput(context.Background(), service, root, []string{"uninstall", "terse-reviewer"}, approve(), &output, &errOutput)
	if code != ExitOK {
		t.Fatalf("uninstall exit=%d stdout=%s stderr=%s", code, output.String(), errOutput.String())
	}
	if !strings.Contains(output.String(), "uninstalled terse-reviewer") {
		t.Fatalf("uninstall output=%q", output.String())
	}
	for _, path := range []string{
		filepath.Join(styleHome, "style.lock.yaml"),
		filepath.Join(styleHome, "build", "index.json"),
		filepath.Join(styleHome, "packages", "terse-reviewer"),
		filepath.Join(pluginDir, ".codex-plugin", "plugin.json"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("uninstall left %s: %v", path, err)
		}
	}
	if got, err := os.ReadFile(configPath); err != nil || !bytes.Equal(got, beforeExternal) {
		t.Fatalf("external config changed: %q, err=%v", got, err)
	}
}

func TestUninstallCodexConfigWithoutGeneratedNewlineResidue(t *testing.T) {
	for _, route := range fixtureRoutes(t, []string{"codex-plugin", "codex-user"}) {
		for _, original := range []struct {
			name string
			data []byte
		}{
			{name: "absent"},
			{name: "user-newline", data: []byte("\n")},
			{name: "user-whitespace", data: []byte(" \r\n\t")},
			{name: "user-config", data: []byte("# Keep user bytes\nmodel = 'keep'")},
		} {
			t.Run(route+"/"+original.name, func(t *testing.T) {
				root := t.TempDir()
				home, config := filepath.Join(root, "style"), filepath.Join(root, "codex")
				service := testCodexInstallService(t, config, filepath.Join(home, "plugin"))
				path := filepath.Join(config, "config.toml")
				if original.data != nil {
					if err := os.MkdirAll(config, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, original.data, 0o640); err != nil {
						t.Fatal(err)
					}
				}
				source := writeInstallPackage(t, filepath.Join(root, "source"), "spec: style/v0.3\nname: residue-probe\nversion: 0.1.0\ndescription: Codex uninstall residue probe\nrules:\n  - id: context\n    target: system\n    text: Keep responses concise.\n")
				otherRoute := "codex-user"
				if route == otherRoute {
					otherRoute = "codex-plugin"
				}
				for _, selected := range fixtureRoutes(t, []string{route, route, otherRoute, route}) {
					if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: home, Harnesses: []string{"codex"}, Routes: map[string]string{"codex": selected}, Yes: true}); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := service.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if original.data == nil {
					if !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("created config survived uninstall: bytes=%q error=%v", got, err)
					}
				} else if err != nil || !bytes.Equal(got, original.data) {
					t.Fatalf("original config changed: got=%q want=%q error=%v", got, original.data, err)
				} else if info, err := os.Stat(path); err != nil || info.Mode().Perm() != fixtureMode(0o640, false) {
					t.Fatalf("original config mode changed: %v %v", info, err)
				}
			})
		}
	}
}

func TestUninstallRequiresConfirmationAndKeepsRemainingPackage(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(root, "plugin"))
	first := filepath.Join(repositoryRoot(t), "testdata", "packages", "terse-reviewer")
	second := writeInstallPackage(t, filepath.Join(root, "second"), "spec: style/v0.3\nname: second\nversion: 0.1.0\ndescription: second\nrules:\n  - id: base\n    target: system\n    text: second\n")
	if _, err := service.Install(context.Background(), InstallRequest{Source: first, Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), InstallRequest{Source: second, Home: styleHome, Harnesses: []string{"codex"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Uninstall(context.Background(), UninstallRequest{Name: "terse-reviewer", Home: styleHome}); err == nil {
		t.Fatal("uninstall without confirmation succeeded")
	}
	result, err := service.Uninstall(context.Background(), UninstallRequest{Name: "terse-reviewer", Home: styleHome, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Remaining) != 1 || result.Remaining[0] != "second" {
		t.Fatalf("remaining = %#v", result.Remaining)
	}
	if _, err := os.Stat(filepath.Join(styleHome, "packages", "second", "STYLE.yaml")); err != nil {
		t.Fatalf("remaining package removed: %v", err)
	}
	lock, err := os.ReadFile(filepath.Join(styleHome, "style.lock.yaml"))
	if err != nil || !bytes.Contains(lock, []byte("name: second")) || bytes.Contains(lock, []byte("name: terse-reviewer")) || !bytes.Contains(lock, []byte("config_dir:")) {
		t.Fatalf("lock after partial uninstall=%q, err=%v", lock, err)
	}
}

func TestRunUninstallAllPreservesLinkedSource(t *testing.T) {
	root := t.TempDir()
	styleHome := filepath.Join(root, "style")
	t.Setenv("STYLE_HOME", styleHome)
	source := writeInstallPackage(t, filepath.Join(root, "linked-source"), "spec: style/v0.3\nname: linked\nversion: 0.1.0\ndescription: linked\nrules:\n  - id: base\n    target: system\n    text: linked\n")
	service := testCodexInstallService(t, filepath.Join(root, "codex"), filepath.Join(styleHome, "plugin"))
	if _, err := service.Install(context.Background(), InstallRequest{Source: source, Home: styleHome, Harnesses: []string{"codex"}, Link: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	var output, errOutput bytes.Buffer
	code := RunWithServiceInput(context.Background(), service, root, []string{"uninstall", "--all", "--json"}, approve(), &output, &errOutput)
	if code != ExitOK {
		t.Fatalf("uninstall --all exit=%d stdout=%s stderr=%s", code, output.String(), errOutput.String())
	}
	var report struct {
		Removed []string `json:"removed"`
		Changed bool     `json:"changed"`
	}
	if err := decodeReport(output.Bytes(), &report); err != nil || len(report.Removed) != 1 || report.Removed[0] != "linked" || !report.Changed {
		t.Fatalf("uninstall --all JSON = %q, decoded=%#v, err=%v", output.String(), report, err)
	}
	if _, err := os.Stat(filepath.Join(source, "STYLE.yaml")); err != nil {
		t.Fatalf("uninstall removed linked source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(styleHome, "style.lock.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uninstall --all left lock file: %v", err)
	}
}
