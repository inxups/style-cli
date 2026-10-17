package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
)

func a8Setup(t *testing.T) (*Service, string, string, string) {
	requirePOSIXFixture(t, "project/plugin delivery requires POSIX")
	t.Helper()
	s, home, config, source := a4Setup(t, "claude-user", strings.Replace(nativePackageYAML("plugin-rules"), "rules:", "rules:\n  - {id: static, target: system, text: 'External static guidance.'}", 1), false)

	return s, home, config, source
}
func a8Install(t *testing.T, s *Service, home, source string) InstallResult {
	t.Helper()
	r, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Routes: map[string]string{"claude-code": "claude-plugin"}, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func a8Registration(t *testing.T, report adapter.RegistrationReport, config, state string) {
	t.Helper()
	market := strings.SplitN(report.Plugin, "@", 2)[1]
	b, _ := json.Marshal(map[string]any{market: map[string]any{"source": map[string]string{"source": "directory", "path": report.Marketplace}, "installLocation": report.Marketplace}})
	writeSyncFile(t, filepath.Join(config, "plugins/known_marketplaces.json"), string(b))
	b, _ = json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{report.Plugin: []any{map[string]string{"scope": "user", "version": "0.1.0", "installPath": filepath.Join(config, "plugins/cache", market, "style-runtime/0.1.0")}}}})
	writeSyncFile(t, filepath.Join(config, "plugins/installed_plugins.json"), string(b))
	var settings map[string]any
	p := filepath.Join(config, "settings.json")
	raw, _ := os.ReadFile(p)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			t.Fatal(err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings["enabledPlugins"] = map[string]bool{report.Plugin: state == "enabled"}
	b, _ = json.Marshal(settings)
	writeSyncFile(t, p, string(b))
}
func TestM3A8PluginLifecycleAndRegistration(t *testing.T) {
	s, home, config, source := a8Setup(t)
	result := a8Install(t, s, home, source)
	if !result.Changed || len(result.Registration) != 1 || result.Registration[0].State != "registration-required" {
		t.Fatal(result)
	}
	lock, err := loadLock(home)
	if err != nil || lock.Format != 3 || lock.Project != nil {
		t.Fatal(lock, err)
	}
	market := filepath.Join(home, "adapters/claude/marketplace")
	plugin := filepath.Join(market, "plugins/style-runtime")
	for _, name := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "scripts/style"} {
		if _, err := os.Stat(filepath.Join(plugin, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(plugin, "bin/style")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if !bytes.Contains(readSyncFile(t, filepath.Join(config, "rules/style-plugin-rules.md")), []byte("External static guidance.")) {
		t.Fatal("missing external static")
	}
	again := a8Install(t, s, home, source)
	if again.Changed {
		t.Fatal(again)
	}
	doctor, err := s.Doctor(context.Background(), DoctorRequest{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range doctor.Diagnostics {
		found = found || d.Code == "registration-required"
	}
	if !found {
		t.Fatal(doctor.Diagnostics)
	}
	a8Registration(t, result.Registration[0], config, "enabled")
	nativeBefore := a4State(t, filepath.Join(config, "plugins"), config)
	sync, err := s.Sync(context.Background(), SyncRequest{Home: home})
	if err != nil || sync.Changed {
		t.Fatal(sync, err)
	}
	a4AssertState(t, nativeBefore, filepath.Join(config, "plugins"), config)
	if _, err := s.Update(context.Background(), UpdateRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a1AssertProvenance(t, home)
	if _, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
		t.Fatal(err)
	}
	before := a4State(t, home, config)
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err == nil || !strings.Contains(err.Error(), "registration-required") {
		t.Fatal(err)
	}
	a4AssertState(t, before, home, config)
	a8Registration(t, result.Registration[0], config, "disabled")
	nativeDB := a4State(t, filepath.Join(config, "plugins"), filepath.Join(config, "plugins"))
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a4AssertState(t, nativeDB, filepath.Join(config, "plugins"), filepath.Join(config, "plugins"))
	for _, p := range []string{filepath.Join(plugin, "scripts/style"), filepath.Join(market, ".claude-plugin/marketplace.json"), filepath.Join(config, "rules/style-plugin-rules.md")} {
		if _, err = os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatal(p, err)
		}
	}
}
func TestM3A8UserPluginTransitionAndNativeGuard(t *testing.T) {
	s, home, config, source := a8Setup(t)
	if _, err := s.Install(context.Background(), InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	external := readSyncFile(t, filepath.Join(config, "CLAUDE.md"))
	static := readSyncFile(t, filepath.Join(config, "rules/style-plugin-rules.md"))
	result := a8Install(t, s, home, source)
	if b, _ := os.ReadFile(filepath.Join(config, "settings.json")); bytes.Contains(b, []byte("--owner=style")) {
		t.Fatal("duplicate user carrier")
	}
	if !bytes.Equal(static, readSyncFile(t, filepath.Join(config, "rules/style-plugin-rules.md"))) {
		t.Fatal("static changed on switch")
	}
	a8Registration(t, result.Registration[0], config, "enabled")
	request := InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Routes: map[string]string{"claude-code": "claude-user"}, Yes: true}
	before := a4State(t, home, config)
	if _, err := s.Install(context.Background(), request); err == nil {
		t.Fatal("switched active plugin")
	}
	a4AssertState(t, before, home, config)
	a8Registration(t, result.Registration[0], config, "disabled")
	if _, err := s.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	lock, err := loadLock(home)
	if err != nil || lock.Format != 1 || lock.Harnesses["claude-code"].Route != "claude-user" {
		t.Fatal(lock, err)
	}
	if !bytes.Equal(external, readSyncFile(t, filepath.Join(config, "CLAUDE.md"))) {
		t.Fatal("foreign instructions changed")
	}
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
}
func TestM3A8UnknownRegistrationAndConfirmationDrift(t *testing.T) {
	for _, mode := range []string{"bad-db", "registration-drift", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, home, config, source := a8Setup(t)
			if mode == "bad-db" {
				writeSyncFile(t, filepath.Join(config, "plugins/installed_plugins.json"), "{}")
				writeSyncFile(t, filepath.Join(config, "plugins/known_marketplaces.json"), "invalid")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Routes: map[string]string{"claude-code": "claude-plugin"}, Confirm: func(result InstallResult) (bool, error) {
				if mode == "cancel" {
					cancel()
				} else {
					a8Registration(t, result.Registration[0], config, "enabled")
				}
				return true, nil
			}}
			result, err := s.Install(ctx, req)
			if err == nil || result.Changed {
				t.Fatal(result, err)
			}
			if _, err := os.Stat(filepath.Join(home, "adapters/claude/marketplace/plugins/style-runtime/scripts/style")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
		})
	}
}
func TestM3A8PluginCLIRegistrationRequired(t *testing.T) {
	s, home, config, source := a8Setup(t)
	var out, errOut bytes.Buffer
	t.Setenv("STYLE_HOME", home)
	args := []string{"install", source, "--harness=claude-code", "--route=claude-code=claude-plugin", "--yes", "--json"}
	if code := RunWithService(context.Background(), s, filepath.Dir(home), args, &out, &errOut); code != 1 || errOut.Len() != 0 {
		t.Fatal(code, &out, &errOut)
	}
	var report InstallResult
	if json.Unmarshal(out.Bytes(), &report) != nil || !report.Changed || len(report.Registration) != 1 {
		t.Fatal(&out)
	}
	a8Registration(t, report.Registration[0], config, "enabled")
	out.Reset()
	errOut.Reset()
	if code := RunWithService(context.Background(), s, filepath.Dir(home), args, &out, &errOut); code != 0 {
		t.Fatal(code, &out, &errOut)
	}
}
func TestM3A8EachCancellationRestoresPlugin(t *testing.T) {
	s, home, _, source := a8Setup(t)
	ctx := &a4CancelAt{Context: context.Background(), at: 1000000}
	if _, err := s.Install(ctx, InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Routes: map[string]string{"claude-code": "claude-plugin"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for at := 1; at <= ctx.checks; at++ {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			s, home, config, source := a8Setup(t)
			before := a4State(t, home, config)
			fault := &a4CancelAt{Context: context.Background(), at: at}
			result, err := s.Install(fault, InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Routes: map[string]string{"claude-code": "claude-plugin"}, Yes: true})
			if !errors.Is(err, context.Canceled) || result.Changed || !fault.reached {
				t.Fatal(result, err, fault)
			}
			a4AssertState(t, before, home, config)
		})
	}
}

func TestM3A8MultiplePackagesNativeUpdateAndPartialUninstall(t *testing.T) {
	s, home, config, source := a8Setup(t)
	first := a8Install(t, s, home, source)
	a8Registration(t, first.Registration[0], config, "enabled")
	second := writeInstallPackage(t, filepath.Join(filepath.Dir(source), "second"), strings.Replace(nativePackageYAML("second"), "rules:", "rules:\n  - {id: static, target: system, text: 'Second external static.'}", 1))
	if _, err := s.Install(context.Background(), InstallRequest{Home: home, Source: second, Harnesses: []string{"claude-code"}, Yes: true}); err != nil {
		t.Fatal(err)
	}
	raw := readSyncFile(t, filepath.Join(source, "STYLE.yaml"))
	writeSyncFile(t, filepath.Join(source, "STYLE.yaml"), strings.ReplaceAll(string(raw), "printf earlier", "printf updated-native"))
	preview, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "plugin-rules"})
	if err != nil || len(preview.Native) == 0 {
		t.Fatal(preview, err)
	}
	if _, err := s.Update(context.Background(), UpdateRequest{Home: home, Name: "plugin-rules", Yes: true}); err != nil {
		t.Fatal(err)
	}
	hooks := readSyncFile(t, filepath.Join(first.Registration[0].Marketplace, "plugins/style-runtime/hooks/hooks.json"))
	if !bytes.Contains(hooks, []byte("updated-native")) {
		t.Fatal(string(hooks))
	}
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, Name: "plugin-rules", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(first.Registration[0].Marketplace, "plugins/style-runtime/scripts/style")); err != nil {
		t.Fatal(err)
	}
	hooks = readSyncFile(t, filepath.Join(first.Registration[0].Marketplace, "plugins/style-runtime/hooks/hooks.json"))
	if bytes.Contains(hooks, []byte("updated-native")) {
		t.Fatal("removed package native remains")
	}
	if _, err := s.InstallFrozen(context.Background(), FrozenRequest{Home: home, Yes: true}); err != nil {
		t.Fatal(err)
	}
	a8Registration(t, first.Registration[0], config, "disabled")
	if _, err := s.Uninstall(context.Background(), UninstallRequest{Home: home, All: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
}
func TestM3A8FailClosedAndNativeWireMatchesUser(t *testing.T) {
	s, home, config, source := a8Setup(t)
	request := InstallRequest{Home: home, Source: source, Harnesses: []string{"claude-code"}, Routes: map[string]string{"claude-code": "claude-plugin"}, FailClosed: []string{"claude-code"}, Yes: true}
	result, err := s.Install(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(result.Registration[0].Marketplace, "plugins/style-runtime/hooks/hooks.json")
	raw := readSyncFile(t, path)
	if !bytes.Contains(raw, []byte(`"onFailure": "block"`)) {
		t.Fatal(string(raw))
	}
	run := func(session string) string {
		t.Helper()
		args := []string{"--style-home=" + home, "--harness-home=" + config, "--harness-version=2.1.315", "agenthooks", "run", "--provider=claude-code", "--owner=style", "--fail-closed"}
		var out, errOut bytes.Buffer
		payload := `{"hook_event_name":"PreToolUse","session_id":"` + session + `","tool_name":"Bash","tool_input":{"command":"true"}}`
		if code := RunWithInput(context.Background(), args, strings.NewReader(payload), &out, &errOut); code != 0 {
			t.Fatal(code, &out, &errOut)
		}
		return out.String()
	}
	plugin := run("plugin")
	request.Routes["claude-code"] = "claude-user"
	if _, err = s.Install(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if user := run("user"); user != plugin || !strings.Contains(user, "Be careful") {
		t.Fatal(user, plugin)
	}
}
