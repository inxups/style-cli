//go:build darwin || linux

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/inxups/style-cli/internal/probe"
)

// These binaries are copies of this test executable, never real harnesses.
// They exercise kernel process relationships and CLI signals offline; they
// provide no evidence for M3-B1/C1 or model delivery.
type a2ProcessFixture struct {
	Home, Runtime, Codex, Claude, Plugin, CodexConfig, ClaudeConfig, Route, Timeout string
	// PluginRoot is where the harness runs plugin hooks from; Codex runs
	// the registered copy in its cache, not the generated plugin directory.
	PluginRoot string
}

func a2ProcessService(f a2ProcessFixture) (*Service, error) {
	ci := adapter.Installation{Provider: codex.ProviderName, ConfigDir: f.CodexConfig, Version: "0.159.3"}
	li := adapter.Installation{Provider: claude.ProviderName, ConfigDir: f.ClaudeConfig, Version: "2.1.315"}
	cd, ld := codex.DefinitionForInstallation(ci), claude.DefinitionForInstallation(li)
	cf, lf := codex.DefaultFacts(), claude.DefaultFacts()
	cd.Routes = []adapter.Route{codex.NewUserRouteForCommand(f.CodexConfig, f.Runtime, &cf), codex.NewPluginRouteForCommand(f.Plugin, f.CodexConfig, f.Runtime, &cf)}
	ld.Routes = []adapter.Route{claude.NewUserRouteForInstallation(f.ClaudeConfig, li.Version, f.Runtime, &lf)}
	return New(cd, ld)
}

func a2HelperEnvironment(f a2ProcessFixture, mode string) []string {
	data, _ := json.Marshal(f)
	env := []string{}
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		switch key {
		case "STYLE_A2_HELPER", "STYLE_A2_FIXTURE", "STYLE_HOME", "PLUGIN_ROOT", "PATH", "CLAUDE_PID", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "CODEX_SANDBOX":
			continue
		}
		env = append(env, variable)
	}
	env = append(env, "STYLE_A2_HELPER="+mode, "STYLE_A2_FIXTURE="+string(data), "STYLE_HOME="+f.Home, "PATH="+filepath.Join(filepath.Dir(f.Home), "bin"))
	if f.Route == "codex-plugin" {
		env = append(env, "PLUGIN_ROOT="+f.PluginRoot)
	}
	return env
}

// a2RunLive runs only the live part of doctor --live, so the report is the
// probe's own, diagnostics included.
func a2RunLive(service *Service, timeout string) int {
	parsed, err := doctorCommand.parse([]string{"--live", "--timeout=" + timeout})
	var request *ProbeRequest
	if err == nil {
		request, err = liveRequest(parsed, os.Stderr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, runErr := service.Probe(ctx, *request)
	if json.NewEncoder(os.Stdout).Encode(result) != nil || runErr != nil {
		return ExitInvalid
	}
	return ExitOK
}

func TestProbeSubprocessHelper(t *testing.T) {
	mode := os.Getenv("STYLE_A2_HELPER")
	if mode == "" {
		return
	}
	var f a2ProcessFixture
	if err := json.Unmarshal([]byte(os.Getenv("STYLE_A2_FIXTURE")), &f); err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "manager":
		service, err := a2ProcessService(f)
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(a2RunLive(service, f.Timeout))
	case "harness", "version", "invalid-hook":
		if mode == "version" {
			fmt.Fprintln(os.Stdout, "offline version fixture; no hook")
		} else {
			attempts := 1
			if mode == "harness" {
				// Receipt observation is best effort within 5ms. Exercise a
				// bounded sequence of genuine fixture callbacks, as a caller
				// may do after a cold or heavily scheduled invocation loses it.
				attempts = 8
			}
			for attempt := 0; attempt < attempts; attempt++ {
				child := exec.Command(f.Runtime, "-test.run=^TestProbeSubprocessHelper$")
				child.Env = a2HelperEnvironment(f, "hook")
				if mode == "invalid-hook" {
					child.Env = a2HelperEnvironment(f, "decode-error")
				}
				child.Stdout, child.Stderr = os.Stdout, os.Stderr
				if err := child.Run(); err != nil {
					t.Fatal(err)
				}
			}
		}
		fmt.Fprintln(os.Stdout, "offline harness ready")
		_, _ = io.Copy(io.Discard, os.Stdin) // Parent holds the native identity live.
		os.Exit(0)
	case "hook", "decode-error", "direct":
		provider, config := "codex", f.CodexConfig
		if f.Route == "claude-user" {
			provider, config = "claude-code", f.ClaudeConfig
		}
		payload := `{"session_id":"offline-kernel-session","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo safe fixture"},"cwd":"/offline","turn_id":"offline-turn"}`
		if mode == "decode-error" {
			payload = `{"incomplete"`
		}
		args := []string{"--style-home=" + f.Home, "--harness-home=" + config, "--harness-version=2.1.315", "agenthooks", "run", "--provider=" + provider, "--owner=style"}
		code := RunWithInput(context.Background(), args, strings.NewReader(payload), os.Stdout, os.Stderr)
		// The normal runtime never joins its observer. Only this fixture keeps
		// the process alive so the bounded worker can be inspected reliably.
		time.Sleep(40 * time.Millisecond)
		os.Exit(code)
	default:
		t.Fatalf("unknown offline helper %q", mode)
	}
}

func a2ProcessInstall(t *testing.T, route string) a2ProcessFixture {
	t.Helper()
	fixture := a2Install(t, route)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fixture.runtime, fixture.codexExe, fixture.claudeExe} {
		if err = os.WriteFile(path, data, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(fixture.root, "bin")
	if err = os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"codex": fixture.codexExe, "claude": fixture.claudeExe} {
		if err = os.Symlink(path, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	return a2ProcessFixture{fixture.home, fixture.runtime, fixture.codexExe, fixture.claudeExe, fixture.plugin, fixture.codexConfig, fixture.claudeConfig, route, "3s", codexPluginCache(fixture.codexConfig)}
}

type a2ProcessStream struct {
	mu    sync.Mutex
	data  bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (s *a2ProcessStream) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.data.Write(data)
	if s.ready != nil && strings.Contains(s.data.String(), "probe ready:") {
		s.once.Do(func() { close(s.ready) })
	}
	return n, err
}
func (s *a2ProcessStream) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.data.String() }

type a2Child struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr a2ProcessStream
	input  io.WriteCloser
	ready  chan struct{}
	done   chan error
}

func a2StartChild(t *testing.T, f a2ProcessFixture, mode string) *a2Child {
	t.Helper()
	executable := f.Runtime
	if mode == "harness" || mode == "version" || mode == "invalid-hook" {
		executable = f.Codex
		if f.Route == "claude-user" {
			executable = f.Claude
		}
	}
	child := &a2Child{cmd: exec.Command(executable, "-test.run=^TestProbeSubprocessHelper$"), ready: make(chan struct{}), done: make(chan error, 1)}
	child.cmd.Env = a2HelperEnvironment(f, mode)
	input, err := child.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.input = input
	child.cmd.Stdout = &child.stdout
	if mode == "manager" {
		child.stderr.ready = child.ready
	}
	child.cmd.Stderr = &child.stderr
	if err = child.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { child.done <- child.cmd.Wait() }()
	t.Cleanup(func() { child.input.Close(); _ = child.cmd.Process.Kill() })
	return child
}
func (c *a2Child) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-c.done:
		return err
	case <-time.After(12 * time.Second):
		_ = c.cmd.Process.Kill()
		t.Fatal("offline subprocess did not exit")
		return nil
	}
}
func (c *a2Child) waitReady(t *testing.T) {
	t.Helper()
	select {
	case <-c.ready:
	case err := <-c.done:
		t.Fatalf("manager exited before ready: %v: %s %s", err, c.stdout.String(), c.stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("manager not ready")
	}
}
func (c *a2Child) report(t *testing.T) ProbeResult {
	t.Helper()
	var report ProbeResult
	decoder := json.NewDecoder(bytes.NewReader(c.stdout.Bytes()))
	if err := decoder.Decode(&report); err != nil {
		t.Fatalf("invalid final JSON: %v: %s %s", err, c.stdout.String(), c.stderr.String())
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatal("stdout contains more than one report")
	}
	if strings.Count(c.stderr.String(), "probe ready:") != 1 {
		t.Fatalf("readiness: %q", c.stderr.String())
	}
	return report
}

func TestM3A2KernelAncestryOfflineCallbacks(t *testing.T) {
	for _, route := range []string{"codex-user", "codex-plugin", "claude-user"} {
		t.Run(route, func(t *testing.T) {
			f := a2ProcessInstall(t, route)
			manager := a2StartChild(t, f, "manager")
			manager.waitReady(t)
			harness := a2StartChild(t, f, "harness")
			if err := manager.wait(t); err != nil {
				t.Fatalf("manager failed: %v: %s %s", err, manager.stdout.String(), manager.stderr.String())
			}
			report := manager.report(t)
			if report.Status != "observed" || !report.Cleanup.Complete || len(report.Targets) != 1 || report.Targets[0].Origin != "process-linked" {
				t.Fatalf("offline kernel callback: %+v", report)
			}
			harness.input.Close()
			if err := harness.wait(t); err != nil {
				t.Fatalf("harness fixture failed: %v %s", err, harness.stderr.String())
			}
			a2NoLeases(t, f.Home)
		})
	}
}

func TestM3A2KernelOriginAndDecodeRefusals(t *testing.T) {
	for _, mode := range []string{"direct", "version", "invalid-hook"} {
		t.Run(mode, func(t *testing.T) {
			f := a2ProcessInstall(t, "codex-user")
			f.Timeout = "1s"
			manager := a2StartChild(t, f, "manager")
			manager.waitReady(t)
			fixture := a2StartChild(t, f, mode)
			if mode == "direct" {
				_ = fixture.wait(t)
			}
			if err := manager.wait(t); err == nil {
				t.Fatal("non-callback succeeded")
			}
			report := manager.report(t)
			if report.Status != "timed-out" || report.Targets[0].CallbackObserved || !report.Cleanup.Complete {
				t.Fatalf("rejected origin: %+v", report)
			}
			if mode != "direct" {
				fixture.input.Close()
				if err := fixture.wait(t); err != nil {
					t.Fatalf("fixture: %v %s", err, fixture.stderr.String())
				}
			}
			a2NoLeases(t, f.Home)
		})
	}
}

func TestM3A2SignalsAndKilledOwnerRecovery(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(signal.String(), func(t *testing.T) {
			f := a2ProcessInstall(t, "codex-user")
			if signal == syscall.SIGKILL {
				f.Timeout = "1s"
			}
			manager := a2StartChild(t, f, "manager")
			manager.waitReady(t)
			leaseData := readSyncFile(t, filepath.Join(f.Home, "probes", "codex.lease.json"))
			var lease probe.Lease
			if err := json.Unmarshal(leaseData, &lease); err != nil {
				t.Fatal(err)
			}
			if err := manager.cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			if err := manager.wait(t); err == nil {
				t.Fatal("interrupted manager succeeded")
			}
			if signal != syscall.SIGKILL {
				report := manager.report(t)
				if report.Status != "interrupted" || !report.Cleanup.Complete {
					t.Fatalf("signal cleanup: %+v", report)
				}
			} else {
				if _, err := os.Stat(filepath.Join(f.Home, "probes", "codex.lease.json")); err != nil {
					t.Fatal("killed fixture lost lease before recovery")
				}
				if delay := time.Until(lease.Expires) + 10*time.Millisecond; delay > 0 {
					time.Sleep(delay)
				}
				next := a2StartChild(t, f, "manager")
				next.waitReady(t)
				if err := next.wait(t); err == nil {
					t.Fatal("recovery fabricated a callback")
				}
				report := next.report(t)
				if report.Status != "timed-out" || report.RunID == lease.RunID || !report.Cleanup.Complete {
					t.Fatalf("recovery: %+v", report)
				}
				if _, err := os.Stat(filepath.Join(f.Home, "probes", "runs", lease.RunID)); !os.IsNotExist(err) {
					t.Fatal("killed run not recovered")
				}
			}
			a2NoLeases(t, f.Home)
		})
	}
}
