package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/inxups/style-cli/internal/probe"
	"github.com/speakeasy-api/agenthooks"
	"runtime"
)

type a2Fixture struct {
	service                                                                     *Service
	home, root, runtime, codexExe, claudeExe, plugin, codexConfig, claudeConfig string
	now                                                                         time.Time
}
type a2RejectDetector struct{ t *testing.T }

func (d a2RejectDetector) Detect(context.Context) (adapter.Installation, bool, error) {
	d.t.Error("probe invoked harness detection")
	return adapter.Installation{}, false, errors.New("forbidden detector")
}
func a2Install(t *testing.T, selection string) *a2Fixture {
	t.Helper()
	requireFixtureRoute(t, selection)
	root := t.TempDir()
	fixture := &a2Fixture{root: root, home: filepath.Join(root, "style"), runtime: filepath.Join(root, "runtime"), codexExe: filepath.Join(root, "native-codex"), claudeExe: filepath.Join(root, "native-claude"), plugin: filepath.Join(root, "plugin"), codexConfig: filepath.Join(root, "codex"), claudeConfig: filepath.Join(root, "claude"), now: time.Now().UTC()}
	if runtime.GOOS == "windows" {
		fixture.runtime += ".exe"
		fixture.codexExe += ".exe"
		fixture.claudeExe += ".exe"
	}
	for path, value := range map[string]string{fixture.runtime: "fixture runtime\x00" + probe.Protocol, fixture.codexExe: "native codex fixture", fixture.claudeExe: "native claude fixture"} {
		if err := os.WriteFile(path, []byte(value), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	codexInstallation := adapter.Installation{Provider: "codex", ConfigDir: fixture.codexConfig, Version: "0.159.3"}
	claudeInstallation := adapter.Installation{Provider: "claude-code", ConfigDir: fixture.claudeConfig, Version: "2.1.315"}
	cd, cl := codex.DefinitionForInstallation(codexInstallation), claude.DefinitionForInstallation(claudeInstallation)
	cd.Detector = fixedDetector{codexInstallation}
	cl.Detector = fixedDetector{claudeInstallation}
	cf, lf := codex.DefaultFacts(), claude.DefaultFacts()
	cd.Routes = []adapter.Route{codex.NewUserRouteForCommand(fixture.codexConfig, fixture.runtime, &cf), codex.NewPluginRouteForCommand(fixture.plugin, fixture.codexConfig, fixture.runtime, &cf)}
	cl.Routes = []adapter.Route{claude.NewUserRouteForInstallation(fixture.claudeConfig, claudeInstallation.Version, fixture.runtime, &lf)}
	service, err := New(cd, cl)
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	harnesses := []string{"codex"}
	routes := map[string]string{"codex": "codex-user"}
	if selection == "codex-plugin" {
		routes["codex"] = "codex-plugin"
	}
	if selection == "claude-user" {
		harnesses = []string{"claude-code"}
		routes = map[string]string{"claude-code": "claude-user"}
	}
	if selection == "all" {
		harnesses = append(harnesses, "claude-code")
		routes["claude-code"] = "claude-user"
	}
	source := writeInstallPackage(t, filepath.Join(root, "source"), a6Manifest("probe-fixture", 50))
	if _, err = service.Install(context.Background(), InstallRequest{Home: fixture.home, Source: source, Harnesses: harnesses, Routes: routes, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if selection == "codex-plugin" {
		path := filepath.Join(fixture.codexConfig, "config.toml")
		data := readSyncFile(t, path)
		data = append(data, []byte("\n\n[plugins.\"style-runtime@style\"]\nenabled = true\n")...)
		os.WriteFile(path, data, 0o600)
	}
	for key, definition := range service.definitions {
		definition.Detector = a2RejectDetector{t}
		service.definitions[key] = definition
	}
	deps := defaultProbeDependencies()
	deps.now = func() time.Time { return fixture.now }
	deps.lookPath = func(name string) (string, error) {
		if name == "codex" {
			return fixture.codexExe, nil
		}
		if name == "claude" {
			return fixture.claudeExe, nil
		}
		return "", fs.ErrNotExist
	}
	deps.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
		if pid == os.Getpid() {
			return agenthooks.ProcessIdentity{PID: pid, Birth: "owner-birth"}, nil
		}
		path := ""
		if pid == 400 {
			path = fixture.codexExe
		}
		if pid == 401 {
			path = fixture.claudeExe
		}
		if path != "" {
			id, err := agenthooks.ExecutableIdentity(path)
			return agenthooks.ProcessIdentity{PID: pid, ParentPID: 1, Birth: "harness-birth", Executable: path, ExecutableID: id}, err
		}
		return agenthooks.ProcessIdentity{}, agenthooks.ErrProcessGone
	}
	deps.poll = func(ctx context.Context) error { fixture.now = fixture.now.Add(2 * time.Minute); return ctx.Err() }
	service.probeDeps = &deps
	return fixture
}
func (f *a2Fixture) emit(t *testing.T, harness string, change func(*probe.Receipt)) {
	t.Helper()
	data := readSyncFile(t, filepath.Join(f.home, "probes", harness+".lease.json"))
	var lease probe.Lease
	if err := json.Unmarshal(data, &lease); err != nil {
		t.Fatal(err)
	}
	event := lease.Binding.Events[0]
	for _, candidate := range lease.Binding.Events {
		if candidate == "PreToolUse" {
			event = candidate
		}
	}
	pid := 400
	if harness == "claude-code" {
		pid = 401
	}
	receipt := probe.Receipt{Format: 1, Protocol: probe.Protocol, RunID: lease.RunID, Harness: harness, HomeSHA256: lease.Binding.HomeSHA256, HarnessHomeSHA256: lease.Binding.HarnessHomeSHA256, Event: event, SessionSHA256: probe.HashString("SESSION_CANARY\x1b[31m"), AgentSHA256: probe.HashString("AGENT_CANARY"), TurnSHA256: probe.HashString("TURN_CANARY"), ObservedAt: lease.Started, Processes: []probe.Process{{PID: 300, ParentPID: pid, Birth: "hook-birth", ExecutableID: lease.Binding.RuntimeID}, {PID: pid, ParentPID: 1, Birth: "harness-birth", ExecutableID: lease.Binding.HarnessID}}}
	if lease.Binding.Route == "codex-plugin" {
		root, err := filepath.EvalSymlinks(f.plugin)
		if err != nil {
			t.Fatal(err)
		}
		receipt.PluginRootSHA256 = probe.HashString(root)
	}
	if change != nil {
		change(&receipt)
	}
	encoded, _ := json.Marshal(receipt)
	if err := os.WriteFile(filepath.Join(f.home, "probes", lease.ReceiptDir, "00.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
func a2HasCode(result ProbeResult, code string) bool {
	for _, finding := range result.Diagnostics {
		if finding.Code == code {
			return true
		}
	}
	return false
}
func a2NoLeases(t *testing.T, home string) {
	t.Helper()
	for _, name := range []string{"codex", "claude-code"} {
		if _, err := os.Lstat(filepath.Join(home, "probes", name+".lease.json")); !os.IsNotExist(err) {
			t.Fatalf("retained lease %s: %v", name, err)
		}
	}
}

type a2FileState struct {
	data  string
	mode  fs.FileMode
	mtime time.Time
}

func a2Protected(t *testing.T, root string) map[string]a2FileState {
	t.Helper()
	files := map[string]a2FileState{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "probes" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files[path] = a2FileState{string(data), info.Mode(), info.ModTime()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestM3A2TargetsReadOnlyAndSuccessfulCallbacks(t *testing.T) {
	for _, mode := range []string{"codex-user", "codex-plugin", "claude-user", "all", "alias"} {
		t.Run(mode, func(t *testing.T) {
			selection := mode
			if mode == "alias" {
				selection = "claude-user"
			}
			fixture := a2Install(t, selection)
			before := a2Protected(t, fixture.root)
			request := ProbeRequest{Home: fixture.home, Timeout: time.Second}
			if mode == "alias" {
				request.Harness = "claude"
			}
			request.Ready = func(ready ProbeReady) error {
				for _, target := range ready.Targets {
					fixture.emit(t, target.Harness, nil)
				}
				return nil
			}
			result, err := fixture.service.Probe(context.Background(), request)
			if err != nil || result.Status != "observed" || !result.Cleanup.Complete || result.RunID == "" {
				t.Fatalf("probe: %+v %v", result, err)
			}
			expected := 1
			if mode == "all" {
				expected = 2
			}
			if len(result.Targets) != expected {
				t.Fatalf("targets: %+v", result.Targets)
			}
			for _, target := range result.Targets {
				if !target.CallbackObserved || target.Status != "callback-observed" || target.Origin != "process-linked" || target.Execution != "not-assessed" {
					t.Fatalf("target: %+v", target)
				}
			}
			if mode == "all" && result.Targets[0].Harness != "codex" {
				t.Fatal("report order changed")
			}
			if after := a2Protected(t, fixture.root); !reflect.DeepEqual(before, after) {
				t.Fatal("probe changed installation/configuration/package bytes, mode or mtime")
			}
			a2NoLeases(t, fixture.home)
			encoded, _ := json.Marshal(result)
			for _, canary := range []string{"SESSION_CANARY", "AGENT_CANARY", "TURN_CANARY", fixture.root, "Shared denial.", "model_body", "native codex"} {
				if bytes.Contains(encoded, []byte(canary)) {
					t.Fatalf("report leaked %q", canary)
				}
			}
		})
	}
}

func TestM3A2CleanupFailurePreservesObservedTarget(t *testing.T) {
	fixture := a2Install(t, "codex-user")
	lookup := fixture.service.probeDeps.lookup
	var replacement []byte
	fixture.service.probeDeps.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
		identity, err := lookup(pid)
		if pid == 400 && replacement == nil {
			path := filepath.Join(fixture.home, "probes", "codex.lease.json")
			var lease probe.Lease
			if err := json.Unmarshal(readSyncFile(t, path), &lease); err != nil {
				t.Fatal(err)
			}
			lease.RunID = strings.Repeat("e", 32)
			lease.ReceiptDir = "runs/" + lease.RunID + "/codex"
			replacement, _ = json.Marshal(lease)
			if err := os.Rename(path, path+"-previous"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, replacement, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return identity, err
	}
	result, err := fixture.service.Probe(context.Background(), ProbeRequest{Home: fixture.home, Timeout: time.Second, Ready: func(ProbeReady) error { fixture.emit(t, "codex", nil); return nil }})
	if err == nil || result.Status != "error" || result.Cleanup.Complete || len(result.Cleanup.Remaining) < 2 || !a2HasCode(result, "probe-cleanup-failed") || !result.Targets[0].CallbackObserved {
		t.Fatalf("cleanup failure: %+v %v", result, err)
	}
	if !bytes.Equal(readSyncFile(t, filepath.Join(fixture.home, "probes", "codex.lease.json")), replacement) {
		t.Fatal("cleanup deleted replacement lease")
	}
}

func TestM3A2CancellationDuringReceiptValidation(t *testing.T) {
	fixture := a2Install(t, "codex-user")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lookup := fixture.service.probeDeps.lookup
	fixture.service.probeDeps.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
		identity, err := lookup(pid)
		if pid == 400 {
			cancel()
			identity.Birth = "changed"
		}
		return identity, err
	}
	result, err := fixture.service.Probe(ctx, ProbeRequest{Home: fixture.home, Timeout: time.Second, Ready: func(ProbeReady) error { fixture.emit(t, "codex", nil); return nil }})
	if err == nil || result.Status != "interrupted" || !result.Cleanup.Complete || a2HasCode(result, "probe-timeout") {
		t.Fatalf("cancellation became timeout: %+v %v", result, err)
	}
	a2NoLeases(t, fixture.home)
}

func TestM3A2ProbePreflightRefusals(t *testing.T) {
	for _, mode := range []string{"missing-home", "missing-lock", "bad-lock", "empty-packages", "uninstalled", "unknown", "auto", "list", "duration", "missing-route", "no-port", "missing-config", "bad-config", "command-drift", "old-runtime", "native-missing", "process-permission", "no-runtime-hook", "plugin-disabled", "home-symlink", "lock-symlink"} {
		t.Run(mode, func(t *testing.T) {
			route := "codex-user"
			if mode == "no-runtime-hook" {
				route = "claude-user"
			}
			if mode == "plugin-disabled" {
				route = "codex-plugin"
			}
			f := a2Install(t, route)
			request := ProbeRequest{Home: f.home, Timeout: time.Second}
			expected := ""
			switch mode {
			case "missing-home":
				request.Home = filepath.Join(t.TempDir(), "absent")
				expected = "probe-not-installed"
			case "missing-lock":
				os.Remove(filepath.Join(f.home, "style.lock.yaml"))
				expected = "probe-not-installed"
			case "bad-lock":
				os.WriteFile(filepath.Join(f.home, "style.lock.yaml"), []byte("["), 0o600)
				expected = "probe-not-installed"
			case "empty-packages":
				lock, _ := loadLock(f.home)
				lock.Packages = nil
				writeLock(f.home, lock)
				expected = "probe-not-installed"
			case "uninstalled":
				request.Harness = "claude-code"
				expected = "probe-not-installed"
			case "unknown":
				request.Harness = "other"
				expected = "runtime-unavailable"
			case "auto":
				request.Harness = "auto"
				expected = "probe-usage"
			case "list":
				request.Harness = "codex,claude-code"
				expected = "probe-usage"
			case "duration":
				request.Timeout = time.Millisecond
				expected = "probe-usage"
			case "missing-route":
				lock, _ := loadLock(f.home)
				item := lock.Harnesses["codex"]
				item.Route = "missing"
				lock.Harnesses["codex"] = item
				writeLock(f.home, lock)
				expected = "runtime-unavailable"
			case "no-port":
				definition := f.service.definitions["codex"]
				definition.Routes = []adapter.Route{staleChangesetRoute{}}
				lock, _ := loadLock(f.home)
				item := lock.Harnesses["codex"]
				item.Route = "test-route"
				lock.Harnesses["codex"] = item
				writeLock(f.home, lock)
				f.service.definitions["codex"] = definition
				expected = "runtime-unavailable"
			case "missing-config":
				os.Remove(filepath.Join(f.codexConfig, "hooks.json"))
				expected = "probe-installation-unavailable"
			case "bad-config":
				os.WriteFile(filepath.Join(f.codexConfig, "hooks.json"), []byte("{"), 0o600)
				expected = "probe-installation-unavailable"
			case "command-drift":
				path := filepath.Join(f.codexConfig, "hooks.json")
				data := readSyncFile(t, path)
				data = fixtureChangedHook(t, data)
				os.WriteFile(path, data, 0o600)
				expected = "probe-installation-unavailable"
			case "old-runtime":
				os.WriteFile(f.runtime, []byte("old runtime without observer"), 0o700)
				expected = "runtime-unavailable"
			case "native-missing":
				f.service.probeDeps.lookPath = func(string) (string, error) { return "", fs.ErrNotExist }
				expected = "origin-unverified"
			case "process-permission":
				f.service.probeDeps.lookup = func(int) (agenthooks.ProcessIdentity, error) { return agenthooks.ProcessIdentity{}, os.ErrPermission }
				expected = "origin-unverified"
			case "no-runtime-hook":
				os.WriteFile(filepath.Join(f.claudeConfig, "settings.json"), []byte(`{"hooks":{}}`), 0o600)
				expected = "probe-no-runtime-hook"
			case "plugin-disabled":
				path := filepath.Join(f.codexConfig, "config.toml")
				data := bytes.ReplaceAll(readSyncFile(t, path), []byte("enabled = true"), []byte("enabled = false"))
				os.WriteFile(path, data, 0o600)
				expected = "probe-installation-unavailable"
			case "home-symlink":
				link := filepath.Join(t.TempDir(), "link")
				os.Symlink(f.home, link)
				request.Home = link
				expected = "probe-not-installed"
			case "lock-symlink":
				path := filepath.Join(f.home, "style.lock.yaml")
				os.Rename(path, path+"-outside")
				os.Symlink(path+"-outside", path)
				expected = "probe-not-installed"
			}
			result, err := f.service.Probe(context.Background(), request)
			if err == nil || !a2HasCode(result, expected) || result.Status != "error" || !result.Cleanup.Complete {
				t.Fatalf("%s: %+v %v expected %s", mode, result, err, expected)
			}
			a2NoLeases(t, f.home)
		})
	}
}

func TestM3A2OriginAndReceiptRejections(t *testing.T) {
	for _, mode := range []string{"direct-call", "runtime-mismatch", "harness-mismatch", "pid-reuse", "permission", "unsupported", "gone", "wrong-root", "wrong-provider", "old-run", "before-ready", "empty-session", "payload-pid", "plugin-root", "user-with-plugin-root", "no-callback"} {
		t.Run(mode, func(t *testing.T) {
			route := "codex-user"
			if mode == "plugin-root" {
				route = "codex-plugin"
			}
			f := a2Install(t, route)
			request := ProbeRequest{Home: f.home, Timeout: time.Second}
			lookup := f.service.probeDeps.lookup
			f.service.probeDeps.lookup = func(pid int) (agenthooks.ProcessIdentity, error) {
				identity, err := lookup(pid)
				if pid == 400 {
					switch mode {
					case "pid-reuse":
						identity.Birth = "reused"
					case "permission":
						err = os.ErrPermission
					case "unsupported":
						err = agenthooks.ErrProcessUnsupported
					case "gone":
						err = agenthooks.ErrProcessGone
					}
				}
				return identity, err
			}
			request.Ready = func(ready ProbeReady) error {
				if mode != "no-callback" {
					f.emit(t, "codex", func(receipt *probe.Receipt) {
						switch mode {
						case "direct-call":
							receipt.Processes = receipt.Processes[:1]
						case "runtime-mismatch":
							receipt.Processes[0].ExecutableID = probe.HashString("other")
						case "harness-mismatch":
							receipt.Processes[1].ExecutableID = probe.HashString("other")
						case "wrong-root":
							receipt.HomeSHA256 = probe.HashString("other")
						case "wrong-provider":
							receipt.Harness = "claude-code"
						case "old-run":
							receipt.RunID = strings.Repeat("a", 32)
						case "before-ready":
							receipt.ObservedAt = receipt.ObservedAt.Add(-time.Second)
						case "empty-session":
							receipt.SessionSHA256 = ""
						case "payload-pid":
							receipt.Processes[1].PID = 999
							receipt.Processes[0].ParentPID = 999
						case "plugin-root":
							receipt.PluginRootSHA256 = probe.HashString("another plugin")
						case "user-with-plugin-root":
							receipt.PluginRootSHA256 = probe.HashString(f.plugin)
						}
					})
				}
				return nil
			}
			result, err := f.service.Probe(context.Background(), request)
			if err == nil || result.Status != "timed-out" || result.Targets[0].CallbackObserved || !result.Cleanup.Complete {
				t.Fatalf("invalid origin accepted %s: %+v %v", mode, result, err)
			}
			if mode == "runtime-mismatch" && !a2HasCode(result, "runtime-mismatch") {
				t.Fatal("runtime mismatch not reported")
			}
			a2NoLeases(t, f.home)
		})
	}
}

func TestM3A2IndexUnavailableAndPartialInterruption(t *testing.T) {
	for _, mode := range []string{"index-missing", "index-invalid", "partial", "interrupted", "partial-interrupted", "readiness-error", "poll-error"} {
		t.Run(mode, func(t *testing.T) {
			selection := "codex-user"
			if strings.HasPrefix(mode, "partial") {
				selection = "all"
			}
			f := a2Install(t, selection)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "index-missing" {
				os.Remove(filepath.Join(f.home, "build", "index.json"))
			}
			if mode == "index-invalid" {
				os.WriteFile(filepath.Join(f.home, "build", "index.json"), []byte(`{"format":99}`), 0o600)
			}
			request := ProbeRequest{Home: f.home, Timeout: time.Second, Ready: func(ready ProbeReady) error {
				if mode == "readiness-error" {
					return io.ErrClosedPipe
				}
				if mode != "interrupted" && mode != "poll-error" {
					f.emit(t, "codex", nil)
				}
				if mode == "interrupted" {
					cancel()
				}
				return nil
			}}
			if mode == "partial-interrupted" {
				f.service.probeDeps.poll = func(context.Context) error { cancel(); return context.Canceled }
			}
			if mode == "poll-error" {
				f.service.probeDeps.poll = func(context.Context) error { return io.ErrUnexpectedEOF }
			}
			result, err := f.service.Probe(ctx, request)
			switch mode {
			case "index-missing", "index-invalid":
				if err != nil || !result.Targets[0].CallbackObserved || !a2HasCode(result, "probe-index-unavailable") {
					t.Fatalf("index prevented callback: %+v %v", result, err)
				}
			case "partial":
				if err == nil || result.Status != "partial" || !result.Targets[0].CallbackObserved || result.Targets[1].CallbackObserved {
					t.Fatalf("partial: %+v %v", result, err)
				}
			case "interrupted", "partial-interrupted":
				if err == nil || result.Status != "interrupted" || !a2HasCode(result, "probe-interrupted") {
					t.Fatalf("interruption: %+v %v", result, err)
				}
				if mode == "partial-interrupted" && !result.Targets[0].CallbackObserved {
					t.Fatal("discarded observed target")
				}
			default:
				if err == nil || !a2HasCode(result, "probe-io-error") {
					t.Fatalf("I/O: %+v %v", result, err)
				}
			}
			if !result.Cleanup.Complete {
				t.Fatal("cleanup incomplete")
			}
			a2NoLeases(t, f.home)
		})
	}
}

func TestM3A2InstallationDriftAndLeaseOwnership(t *testing.T) {
	for _, mode := range []string{"lock", "config", "trust", "runtime", "runtime-same-metadata", "harness", "index", "lease"} {
		t.Run(mode, func(t *testing.T) {
			f := a2Install(t, "codex-user")
			request := ProbeRequest{Home: f.home, Timeout: time.Second, Ready: func(ready ProbeReady) error {
				f.emit(t, "codex", nil)
				path := filepath.Join(f.home, "style.lock.yaml")
				switch mode {
				case "config":
					path = filepath.Join(f.codexConfig, "hooks.json")
				case "trust":
					path = filepath.Join(f.codexConfig, "config.toml")
				case "runtime", "runtime-same-metadata":
					path = f.runtime
				case "harness":
					path = f.codexExe
				case "index":
					path = filepath.Join(f.home, "build", "index.json")
				case "lease":
					path = filepath.Join(f.home, "probes", "codex.lease.json")
				}
				info, _ := os.Stat(path)
				data := readSyncFile(t, path)
				if mode == "runtime-same-metadata" {
					data = bytes.Replace(data, []byte("fixture"), []byte("changed"), 1)
				} else if mode == "lease" {
					os.Rename(path, path+"-other")
				} else {
					data = append(data, ' ')
				}
				if err := os.WriteFile(path, data, info.Mode().Perm()); err != nil {
					return err
				}
				if mode == "runtime-same-metadata" {
					return os.Chtimes(path, info.ModTime(), info.ModTime())
				}
				return nil
			}}
			result, err := f.service.Probe(context.Background(), request)
			expected := "installation-changed"
			if mode == "lease" {
				expected = "probe-lease-changed"
			}
			if err == nil || !a2HasCode(result, expected) || result.Status != "error" {
				t.Fatalf("drift accepted %s: %+v %v", mode, result, err)
			}
			if mode != "lease" {
				if !result.Cleanup.Complete {
					t.Fatal("drift cleanup failed")
				}
				a2NoLeases(t, f.home)
			} else {
				if result.Cleanup.Complete || !a2HasCode(result, "probe-cleanup-failed") {
					t.Fatal("replacement cleanup silently accepted")
				}
				if _, err = os.Stat(filepath.Join(f.home, "probes", "codex.lease.json")); err != nil {
					t.Fatal("removed replacement lease")
				}
			}
		})
	}
}

func TestM3A2CLIUsageJSONAndTerminalSafety(t *testing.T) {
	for _, args := range [][]string{{"--timeout", "0s"}, {"--timeout", "999ms"}, {"--timeout", "11m"}, {"--timeout", "nonsense"}, {"--timeout"}, {"--harness"}, {"--harness", "auto"}, {"--harness", "codex,claude-code"}, {"--json=bad"}, {"--json", "--json"}, {"--unknown"}, {"operand"}, {"--harness="}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := RunWithService(context.Background(), testService(t), t.TempDir(), append([]string{"probe"}, args...), &out, &stderr)
			if code != 1 || out.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("usage %v: %d %q %q", args, code, out.String(), stderr.String())
			}
		})
	}
	for _, duration := range []string{"1s", "10m", "1000ms"} {
		if _, _, err := parseProbeArgs([]string{"--timeout=" + duration}); err != nil {
			t.Fatal(duration, err)
		}
	}
	f := a2Install(t, "codex-user")
	t.Setenv("STYLE_HOME", f.home)
	var out, stderr bytes.Buffer
	code := RunWithService(context.Background(), f.service, f.root, []string{"probe", "--json", "--timeout", "1s"}, &out, &stderr)
	var report ProbeResult
	decoder := json.NewDecoder(&out)
	if code != 1 || decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF || report.Status != "timed-out" || !strings.Contains(stderr.String(), "probe ready:") {
		t.Fatalf("JSON streaming: %d %+v %q", code, report, stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code = RunWithService(context.Background(), f.service, f.root, []string{"probe", "--help"}, &out, &stderr); code != 0 || !strings.Contains(out.String(), probeUsage) || stderr.Len() != 0 {
		t.Fatal("help failed")
	}
	report.Targets = []ProbeTarget{{Harness: "evil\x1b[31m\n", Route: "route\x1b[0m", Status: "unavailable", Origin: "unverified"}}
	report.Diagnostics = []ProbeDiagnostic{{Code: "safe", Severity: "error", Message: "secret\x1b[31m\n"}}
	out.Reset()
	writeProbeText(&out, report)
	if strings.ContainsRune(out.String(), '\x1b') || !strings.Contains(out.String(), `\u001b`) {
		t.Fatal("terminal controls not escaped")
	}
}
