package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

func testManifest() Manifest {
	return Manifest{
		Command: []string{"/usr/local/bin/myhooks"},
		Hooks: []HookSpec{
			{Kind: agenthooks.KindToolPre, Blocking: true, Timeout: 30 * time.Second,
				Tools: ToolMatcher{Names: []string{"Bash"}}},
			{Kind: agenthooks.KindStop, Blocking: false},
			{Kind: agenthooks.KindToolPost, Blocking: false},
		},
		Identity: Identity{Name: "myhooks", Version: "1.0.0", Description: "test hooks"},
		Fail:     agenthooks.FailClosed,
	}
}

func readRendered(t *testing.T, fsys fs.FS, path string) []byte {
	t.Helper()
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("rendered file %s: %v", path, err)
	}
	return b
}

type claudeConfig struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type      string   `json:"type"`
			Command   string   `json:"command"`
			Args      []string `json:"args"`
			Timeout   int      `json:"timeout"`
			Async     bool     `json:"async"`
			OnFailure string   `json:"onFailure"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func TestRenderClaudePlugin(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopePlugin, HarnessVersion: "2.1.295"})
	if err != nil {
		t.Fatal(err)
	}
	var plugin map[string]string
	if err := json.Unmarshal(readRendered(t, fsys, ".claude-plugin/plugin.json"), &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin["name"] != "myhooks" {
		t.Errorf("plugin.json wrong: %v", plugin)
	}

	var cfg claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "hooks/hooks.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	pre := cfg.Hooks["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != "Bash" {
		t.Fatalf("PreToolUse entry wrong: %+v", pre)
	}
	cmd := pre[0].Hooks[0]
	if cmd.Command != "/usr/local/bin/myhooks" || !reflect.DeepEqual(cmd.Args, []string{"agenthooks", "run", "--provider=claude-code", "--owner=agenthooks", "--timeout=30s", "--fail-closed"}) || cmd.Timeout != 30 || cmd.Async || cmd.OnFailure != "block" {
		t.Errorf("PreToolUse command wrong: %+v", cmd)
	}
	// quirk #1: Stop is forced synchronous even for telemetry hooks.
	stop := cfg.Hooks["Stop"]
	if len(stop) != 1 || stop[0].Hooks[0].Async {
		t.Errorf("Stop must render async:false (cowork drops async Stop): %+v", stop)
	}
	if stop[0].Hooks[0].OnFailure != "" || containsArg(stop[0].Hooks[0].Args, "--fail-closed") {
		t.Errorf("Stop telemetry hook must not use fail-closed settings: %+v", stop[0].Hooks[0])
	}
	// Telemetry hooks elsewhere stay async.
	post := cfg.Hooks["PostToolUse"]
	if len(post) != 1 || !post[0].Hooks[0].Async {
		t.Errorf("non-blocking PostToolUse should be async: %+v", post)
	}
	if post[0].Hooks[0].OnFailure != "" || containsArg(post[0].Hooks[0].Args, "--fail-closed") {
		t.Errorf("PostToolUse telemetry hook must not use fail-closed settings: %+v", post[0].Hooks[0])
	}
}

func TestClaudeExecArgsAndFailClosedVersionGate(t *testing.T) {
	m := testManifest()
	m.Command = []string{"/opt/STYLE runtime/bin/style", "--config", "/tmp/config with spaces.json"}
	m.Owner = "style"
	target := Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser, HarnessVersion: "2.1.295"}
	fsys, err := Render(m, target)
	if err != nil {
		t.Fatal(err)
	}
	var cfg claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "settings.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	pre := cfg.Hooks["PreToolUse"][0].Hooks[0]
	wantArgs := []string{"--config", "/tmp/config with spaces.json", "agenthooks", "run", "--provider=claude-code", "--owner=style", "--timeout=30s", "--fail-closed"}
	if pre.Command != "/opt/STYLE runtime/bin/style" || !reflect.DeepEqual(pre.Args, wantArgs) {
		t.Errorf("Claude exec argv = %q %q, want %q %q", pre.Command, pre.Args, "/opt/STYLE runtime/bin/style", wantArgs)
	}

	for _, version := range []string{"2.1.294", "", "unknown", "2.1.295-beta"} {
		name := version
		if name == "" {
			name = "missing"
		}
		t.Run("reject-"+strings.ReplaceAll(name, ".", "_"), func(t *testing.T) {
			badTarget := target
			badTarget.HarnessVersion = version
			if _, err := Render(m, badTarget); err == nil {
				t.Errorf("fail-closed accepted unsupported/unknown Claude version %q", version)
			}
		})
	}
	for _, version := range []string{"2.1.295", "v2.1.295", "2.1.296", "2.1.296 (Claude Code)"} {
		if !claudeSupportsOnFailure(version) {
			t.Errorf("supported Claude version %q rejected", version)
		}
	}

	// Fail-open and non-blocking PreToolUse do not need the version-gated
	// failure mechanism and must not emit its configuration or runtime flag.
	m.Fail = agenthooks.FailOpen
	openFS, err := Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("fail-open render needs no version: %v", err)
	}
	var openCfg claudeConfig
	if err := json.Unmarshal(readRendered(t, openFS, "settings.json"), &openCfg); err != nil {
		t.Fatal(err)
	}
	openPre := openCfg.Hooks["PreToolUse"][0].Hooks[0]
	if openPre.OnFailure != "" || containsArg(openPre.Args, "--fail-closed") {
		t.Errorf("fail-open PreToolUse emitted fail-closed settings: %+v", openPre)
	}
	m.Fail = agenthooks.FailClosed
	m.Hooks = []HookSpec{{Kind: agenthooks.KindToolPre, Blocking: false}}
	if _, err := Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser}); err != nil {
		t.Fatalf("non-blocking PreToolUse should not need onFailure support: %v", err)
	}
	m.Command = []string{"", "arg"}
	if _, err := Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser}); err == nil {
		t.Fatal("Claude exec renderer accepted an empty executable")
	}
}

func TestRenderClaudeTimeoutRoundingAndDefault(t *testing.T) {
	m := testManifest()
	m.Fail = agenthooks.FailOpen
	m.Hooks = []HookSpec{{Kind: agenthooks.KindToolPre, Blocking: false, Timeout: 1500 * time.Millisecond}}
	fsys, err := Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	var cfg claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "settings.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Hooks["PreToolUse"][0].Hooks[0].Timeout; got != 2 {
		t.Fatalf("timeout = %d, want ceil(1.5s) = 2", got)
	}

	m.Hooks = []HookSpec{{Kind: agenthooks.KindSessionStart, Blocking: false}}
	fsys, err = Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(readRendered(t, fsys, "settings.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Hooks["SessionStart"][0].Hooks[0].Timeout; got != 120 {
		t.Fatalf("SessionStart default timeout = %d, want 120", got)
	}
}

func TestRenderClaudeNativeEventsAndMatchers(t *testing.T) {
	manifest := Manifest{
		Command: []string{"/tmp/style"},
		Owner:   "style",
		Hooks: []HookSpec{
			{Kind: agenthooks.KindPromptExpansion, NativeMatcher: `^review$`, Blocking: true},
			{Kind: agenthooks.KindOther, NativeEvent: "PostModelSwitch", Blocking: false},
		},
	}
	fsys, err := Render(manifest, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	var cfg claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "settings.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	review := cfg.Hooks["UserPromptExpansion"]
	if len(review) != 1 || review[0].Matcher != `^review$` || len(review[0].Hooks) != 1 || review[0].Hooks[0].Async {
		t.Errorf("native prompt-expansion matcher rendered incorrectly: %#v", review)
	}
	switchEvent := cfg.Hooks["PostModelSwitch"]
	if len(switchEvent) != 1 || len(switchEvent[0].Hooks) != 1 || !switchEvent[0].Hooks[0].Async {
		t.Errorf("direct native observer event rendered incorrectly: %#v", switchEvent)
	}
}

func TestRenderClaudeRejectsInvalidNativeMatcher(t *testing.T) {
	manifest := Manifest{
		Command: []string{"/tmp/style"},
		Hooks:   []HookSpec{{Kind: agenthooks.KindPromptExpansion, NativeMatcher: "[", Blocking: true}},
	}
	if _, err := Render(manifest, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser}); err == nil || !strings.Contains(err.Error(), "invalid native Claude matcher") {
		t.Fatalf("invalid native matcher error = %v", err)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestPlanFilesReturnsMergedChangesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: dir}
	manifest := testManifest()
	manifest.Owner = "style"
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(`{"hooks":{"Other":[{"hooks":[{"type":"command","command":"other-tool"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model = \"o4\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, err := PlanFiles(manifest, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("PlanFiles returned %d changes, want config, hooks and restoration record", len(changes))
	}
	for _, change := range changes {
		if change.Path == filepath.Join(dir, restorePath("hooks.json", "style")) {
			if change.Before != nil || len(change.After) == 0 || change.Mode != 0o600 {
				t.Error("invalid private restoration plan")
			}
			continue
		}
		if change.Path != filepath.Join(dir, "hooks.json") && change.Path != filepath.Join(dir, "config.toml") {
			t.Errorf("unexpected planned path %q", change.Path)
		}
		if len(change.Before) == 0 || len(change.After) == 0 {
			t.Errorf("planned change omitted bytes: %#v", change)
		}
		if change.Remove {
			t.Errorf("install plan unexpectedly removes %q", change.Path)
		}
	}
	hooks, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(hooks) != `{"hooks":{"Other":[{"hooks":[{"type":"command","command":"other-tool"}]}]}}` {
		t.Fatalf("PlanFiles modified hooks.json: %s", hooks)
	}
	for _, change := range changes {
		if !strings.Contains(string(change.After), "other-tool") && filepath.Base(change.Path) == "hooks.json" {
			t.Errorf("merged hooks omitted foreign handler: %s", change.After)
		}
	}
}

func TestApplyFilesRejectsStalePlanAndAppliesFreshPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	changes := []PlannedFile{{Path: path, Root: dir, After: []byte("new\n"), Mode: 0o644}}
	if err := ApplyFiles(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "new\n" {
		t.Fatalf("applied plan = %q, err=%v", got, err)
	}
	stale := PlannedFile{Path: path, Root: dir, Before: []byte("old\n"), After: []byte("newer\n"), Mode: 0o644}
	if err := ApplyFiles(context.Background(), []PlannedFile{stale}); err == nil {
		t.Fatal("ApplyFiles accepted a stale plan")
	}
}

type cancelAfterChecks struct {
	context.Context
	checks int
	cancel int
}

func (ctx *cancelAfterChecks) Err() error {
	ctx.checks++
	if ctx.checks >= ctx.cancel {
		return context.Canceled
	}
	return nil
}

func TestApplyFilesRollsBackEarlierChangesWhenContextCancels(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	if err := os.WriteFile(first, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterChecks{Context: context.Background(), cancel: 6}
	changes := []PlannedFile{
		{Path: first, Root: dir, Before: []byte("before"), After: []byte("after"), Mode: 0o600},
		{Path: second, Root: dir, After: []byte("created"), Mode: 0o600},
	}
	if err := ApplyFiles(ctx, changes); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyFiles error = %v, want context cancellation", err)
	}
	if got, err := os.ReadFile(first); err != nil || string(got) != "before" {
		t.Fatalf("first file after rollback = %q, err=%v", got, err)
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("second file exists after rollback, err=%v", err)
	}
	if mode, err := os.Stat(first); err != nil || mode.Mode().Perm() != fixtureMode(0o600) {
		t.Fatalf("rollback changed original mode: mode=%v err=%v", mode, err)
	}
}

func TestPlanUninstallFilesReturnsOwnerScopedChanges(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: dir}
	manifest := testManifest()
	manifest.Owner = "style"
	if err := Install(context.Background(), manifest, target); err != nil {
		t.Fatal(err)
	}
	changes, err := PlanUninstallFiles(manifest, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("PlanUninstallFiles returned %d changes, want 2: %#v", len(changes), changes)
	}
	for _, change := range changes {
		if len(change.Before) == 0 {
			t.Errorf("uninstall plan omitted before bytes: %#v", change)
		}
		if !change.Remove && len(change.After) == 0 {
			t.Errorf("uninstall update omitted after bytes: %#v", change)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(context.Background(), manifest, target); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hooks.json", "config.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s remains after uninstall, err=%v", name, err)
		}
	}
}

func TestRenderCodexUsesAndValidatesNativeMatcher(t *testing.T) {
	manifest := Manifest{
		Command: []string{"/usr/local/bin/style"},
		Hooks:   []HookSpec{{Kind: agenthooks.KindToolPre, NativeMatcher: `^(Bash|apply_patch|mcp__.*)$`, Blocking: true}},
		Owner:   "style",
	}
	fsys, err := Render(manifest, Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var config claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "hooks.json"), &config); err != nil {
		t.Fatal(err)
	}
	entries := config.Hooks["PreToolUse"]
	if len(entries) != 1 || entries[0].Matcher != `^(Bash|apply_patch|mcp__.*)$` {
		t.Fatalf("native Codex matcher = %#v", entries)
	}
	manifest.Hooks[0].NativeMatcher = "["
	if _, err := Render(manifest, Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: t.TempDir()}); err == nil {
		t.Fatal("invalid native Codex matcher was accepted")
	}
}

func TestRenderCursorFailClosed(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderCursor, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Version int `json:"version"`
		Hooks   map[string][]struct {
			Command    string `json:"command"`
			Timeout    int    `json:"timeout"`
			FailClosed bool   `json:"failClosed"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(readRendered(t, fsys, ".cursor/hooks.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d", cfg.Version)
	}
	// tool.pre expands to the specific + generic events (runner dedupes).
	for _, event := range []string{"beforeShellExecution", "beforeMCPExecution", "beforeReadFile", "preToolUse"} {
		entries := cfg.Hooks[event]
		if len(entries) != 1 {
			t.Fatalf("%s missing", event)
		}
		if !entries[0].FailClosed {
			t.Errorf("%s: decision hooks must render failClosed (quirk #7)", event)
		}
		// Cursor can't express matchers: the filter must ride the argv.
		if !strings.Contains(entries[0].Command, "--filter=") {
			t.Errorf("%s: matcher must compile to --filter: %s", event, entries[0].Command)
		}
	}
	if entries := cfg.Hooks["stop"]; len(entries) != 1 || entries[0].FailClosed {
		t.Errorf("telemetry stop hook must stay fail-open: %+v", entries)
	}
}

// quirk #29: a scheme:// URL in any command makes Cursor silently drop the
// whole hooks.json, so rendering one must fail loudly instead.
func TestRenderCursorRejectsURLInCommand(t *testing.T) {
	m := testManifest()
	m.Command = []string{"/usr/local/bin/myhooks", "--server=https://example.com"}
	_, err := Render(m, Target{Provider: agenthooks.ProviderCursor, Scope: ScopeProject})
	if err == nil || !strings.Contains(err.Error(), "quirk #29") {
		t.Fatalf("expected quirk #29 rejection, got %v", err)
	}
	// Other providers accept URLs in commands.
	if _, err := Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopePlugin, HarnessVersion: "2.1.300"}); err != nil {
		t.Fatalf("claude render should accept URL commands: %v", err)
	}
}

func TestRenderGeminiMilliseconds(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderGemini, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
				Timeout int64  `json:"timeout"`
				Name    string `json:"name"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(readRendered(t, fsys, ".gemini/settings.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	pre := cfg.Hooks["BeforeTool"]
	if len(pre) != 1 || pre[0].Hooks[0].Timeout != 30000 {
		t.Errorf("gemini timeout must be milliseconds (quirk #14): %+v", pre)
	}
	if pre[0].Hooks[0].Name != "myhooks:tool.pre" {
		t.Errorf("gemini hooks need names for /hooks UX: %+v", pre[0].Hooks[0])
	}
}

func TestRenderCodexAsyncAndTrust(t *testing.T) {
	m := testManifest()
	m.Hooks = append(m.Hooks, HookSpec{Kind: agenthooks.KindSessionEnd, Blocking: false, Timeout: 60 * time.Second})
	fsys, err := Render(m, Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: "/codex-home"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "hooks.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	// Codex parses-but-skips async (quirk #10): telemetry hooks get --async
	// so the runner detaches itself; no shell wrapper anywhere.
	post := cfg.Hooks["PostToolUse"][0].Hooks[0]
	command := post.Command
	if runtime.GOOS == "windows" {
		decoded, ok := windowsDecodedCommand(command)
		if !ok {
			t.Fatal("invalid Windows command")
		}
		command = decoded
	}
	if !strings.Contains(command, "--async") || strings.Contains(command, "/bin/sh") {
		t.Errorf("non-blocking codex hook must render --async without a shell: %s", post.Command)
	}
	preEntry := cfg.Hooks["PreToolUse"][0]
	pre := preEntry.Hooks[0]
	if strings.Contains(pre.Command, "--async") {
		t.Errorf("blocking codex hook must stay synchronous: %s", pre.Command)
	}
	sessionEndEntry := cfg.Hooks["SessionEnd"][0]
	sessionEnd := sessionEndEntry.Hooks[0]
	if sessionEnd.Timeout != 3 || !strings.HasSuffix(sessionEnd.Command, " --async") {
		t.Errorf("SessionEnd must detach within Codex's 3-second teardown budget: %+v", sessionEnd)
	}

	// Trust state keys are "<CODEX_HOME>/hooks.json:<event_label>:<group>:<handler>"
	// and land in config.toml inside the managed marker region. The source
	// path is OS-native (filepath.Join, like Codex's own canonicalization),
	// so build the expectation the same way and TOML-escape it.
	trust := string(readRendered(t, fsys, "config.toml"))
	source := filepath.Join("/codex-home", "hooks.json")
	if !strings.Contains(trust, `[hooks.state.`+tomlString(source+":pre_tool_use:0:0")+`]`) {
		t.Errorf("trust seeding missing pre_tool_use state key:\n%s", trust)
	}
	if !strings.Contains(trust, `[hooks.state.`+tomlString(source+":stop:0:0")+`]`) {
		t.Errorf("trust seeding missing stop state key:\n%s", trust)
	}
	if !strings.Contains(trust, `[hooks.state.`+tomlString(source+":session_end:0:0")+`]`) {
		t.Errorf("trust seeding missing session_end state key:\n%s", trust)
	}
	wantHash := DefinitionHash("PreToolUse", preEntry.Matcher, pre.Command, pre.Timeout)
	if !strings.Contains(trust, `trusted_hash = "`+wantHash+`"`) {
		t.Errorf("trust file must contain the definition hash %s:\n%s", wantHash, trust)
	}
	wantSessionEndHash := DefinitionHash("SessionEnd", sessionEndEntry.Matcher, sessionEnd.Command, sessionEnd.Timeout)
	if !strings.Contains(trust, `trusted_hash = "`+wantSessionEndHash+`"`) {
		t.Errorf("trust file must contain the SessionEnd definition hash %s:\n%s", wantSessionEndHash, trust)
	}
	if !strings.Contains(trust, "BEGIN agenthooks managed hooks") || !strings.Contains(trust, "END agenthooks managed hooks") {
		t.Errorf("codex config.toml must use the managed marker region:\n%s", trust)
	}
}

func TestRenderAndInstallCodexPlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Codex plugin launcher is POSIX shell")
	}
	m := testManifest()
	m.Owner = "style"
	m.Identity = Identity{Name: "style-runtime", Version: "0.3.0", Description: "STYLE runtime", PluginID: "style-runtime@style"}
	m.Hooks[0].Tools = ToolMatcher{Canonical: []agenthooks.CanonicalTool{agenthooks.ToolShell}}
	pluginDir, codexHome := t.TempDir(), t.TempDir()
	target := Target{Provider: agenthooks.ProviderCodex, Scope: ScopePlugin, Dir: pluginDir, ConfigDir: codexHome}

	fsys, err := Render(m, target)
	if err != nil {
		t.Fatal(err)
	}
	var plugin struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(readRendered(t, fsys, ".codex-plugin/plugin.json"), &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin.Name != m.Identity.Name || plugin.Version != m.Identity.Version || plugin.Description != m.Identity.Description {
		t.Errorf("plugin metadata = %+v", plugin)
	}
	var cfg claudeConfig
	if err := json.Unmarshal(readRendered(t, fsys, "hooks/hooks.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	pre := cfg.Hooks["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != "Bash" || len(pre[0].Hooks) != 1 {
		t.Fatalf("plugin PreToolUse = %+v", pre)
	}
	command := pre[0].Hooks[0].Command
	if want := `exec "$PLUGIN_ROOT/bin/style" agenthooks run --provider=codex --owner=style --timeout=30s`; command != want {
		t.Errorf("plugin hook command = %q, want %q", command, want)
	}
	if strings.Contains(string(readRendered(t, fsys, "hooks/hooks.json")), `"async": true`) {
		t.Error("blocking plugin hook must not be async")
	}
	trust := string(readRendered(t, fsys, "config.toml"))
	trustKey := "style-runtime@style:hooks/hooks.json:pre_tool_use:0:0"
	if !strings.Contains(trust, `[hooks.state.`+tomlString(trustKey)+`]`) {
		t.Errorf("plugin trust key missing from CODEX_HOME config.toml:\n%s", trust)
	}
	wantHash := DefinitionHash("PreToolUse", pre[0].Matcher, command, pre[0].Hooks[0].Timeout)
	if !strings.Contains(trust, `trusted_hash = "`+wantHash+`"`) {
		t.Errorf("plugin trusted hash %q missing:\n%s", wantHash, trust)
	}
	launcher := readRendered(t, fsys, "bin/style")
	if !strings.HasPrefix(string(launcher), "#!/bin/sh\nexec /usr/local/bin/myhooks \"$@\"\n") {
		t.Errorf("launcher = %q", launcher)
	}
	launcherInfo, err := fs.Stat(fsys, "bin/style")
	if err != nil {
		t.Fatal(err)
	}
	if got := launcherInfo.Mode().Perm(); got != 0o755 {
		t.Errorf("rendered launcher mode = %04o, want 0755", got)
	}

	configPath := filepath.Join(codexHome, "config.toml")
	otherConfig := "# keep this setting\n" + tomlBeginMarker + "\n[hooks.state.\"legacy:key\"]\ntrusted_hash = \"legacy\"\n" + tomlEndMarker + "\n"
	if err := os.WriteFile(configPath, []byte(otherConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	installedLauncher := filepath.Join(pluginDir, "bin", "style")
	info, err := os.Stat(installedLauncher)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("installed launcher mode = %04o, want 0755", got)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("trust config unexpectedly installed under plugin root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "config.toml")); err != nil {
		t.Errorf("trust config not installed under CODEX_HOME: %v", err)
	}
	changes, err := Diff(m, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.State != StateUnchanged {
			t.Errorf("repeat plugin install is not idempotent: %+v", change)
		}
	}
	if err := Uninstall(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(pluginDir, ".codex-plugin", "plugin.json"),
		filepath.Join(pluginDir, "hooks", "hooks.json"),
		filepath.Join(pluginDir, "bin", "style"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("plugin artifact remains after uninstall: %s (err=%v)", path, err)
		}
	}
	configAfterUninstall, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configAfterUninstall), "# keep this setting") || !strings.Contains(string(configAfterUninstall), "legacy:key") || strings.Contains(string(configAfterUninstall), "owner=style") {
		t.Errorf("plugin uninstall did not preserve the other trust region:\n%s", configAfterUninstall)
	}
}

func TestMergeManagedJSONReplacesOnlyMatchingOwner(t *testing.T) {
	m := testManifest()
	m.Owner = "style"
	renderedFS, err := Render(m, Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser, HarnessVersion: "2.1.300"})
	if err != nil {
		t.Fatal(err)
	}
	rendered := readRendered(t, renderedFS, "settings.json")
	existing := []byte(`{"env":{"KEEP":"yes"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/old/style","args":["agenthooks","run","--owner=style"]},{"type":"command","command":"other-agenthooks run --owner=agenthooks"},{"type":"command","command":"other-agenthooks check"}]}]}}`)
	merged, err := mergeManagedJSON(existing, rendered, "style")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Env   map[string]string `json:"env"`
		Hooks map[string][]struct {
			Hooks []struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got.Env["KEEP"] != "yes" {
		t.Errorf("unrelated config lost: %+v", got.Env)
	}
	var style, otherOwner, unmarked int
	for _, group := range got.Hooks["PreToolUse"] {
		for _, handler := range group.Hooks {
			switch {
			case handlerHasOwner(handler.Command, handler.Args, "style"):
				style++
				if handler.Command == "/old/style" {
					t.Errorf("stale STYLE command survived: %+v", handler)
				}
			case handlerHasOwner(handler.Command, handler.Args, "agenthooks"):
				otherOwner++
			case strings.Contains(handler.Command, "other-agenthooks check"):
				unmarked++
			}
		}
	}
	if style != 1 || otherOwner != 1 || unmarked != 1 {
		t.Errorf("owner merge counts style=%d other-owner=%d unmarked=%d; merged=%s", style, otherOwner, unmarked, merged)
	}
}

func TestUninstallRemovesOnlyMatchingOwner(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser, Dir: dir, HarnessVersion: "2.1.300"}
	settingsPath := filepath.Join(dir, "settings.json")
	foreign := []byte(`{"env":{"KEEP":"yes"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"other-agenthooks check"},{"type":"command","command":"agenthooks run --owner=agenthooks"}]}]}}`)
	if err := os.WriteFile(settingsPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	m := testManifest()
	m.Owner = "style"
	if err := Install(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	target.HarnessVersion = ""
	if err := Uninstall(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, foreign) {
		t.Errorf("uninstall removed unrelated entries:\n%s", got)
	}
	if bytes.Contains(got, []byte("--owner=style")) || bytes.Contains(got, []byte("old-style")) {
		t.Errorf("STYLE-owned commands remain after uninstall:\n%s", got)
	}
	if mode, err := os.Stat(settingsPath); err != nil || mode.Mode().Perm() != fixtureMode(0o600) {
		t.Errorf("uninstall changed existing config permissions: mode=%v err=%v", mode, err)
	}
}

func TestUninstallClaudeAfterVersionDowngrade(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeUser, Dir: dir, HarnessVersion: "2.1.300"}
	m := testManifest()
	m.Owner = "style"
	if err := Install(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	target.HarnessVersion = "2.1.294"
	if err := Uninstall(context.Background(), m, target); err != nil {
		t.Fatalf("uninstall must still work after Claude version downgrade: %v", err)
	}
	settings, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if bytes.Contains(settings, []byte("--owner=style")) {
		t.Fatalf("STYLE-owned hook remains after downgrade uninstall:\n%s", settings)
	}
}

func TestRemoveOwnedJSONDetectsChangesInsideExistingGroup(t *testing.T) {
	existing := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"command":"style --owner=style"},{"command":"other --owner=other"}]}]}}`)
	updated, removeFile, changed, err := removeOwnedJSON(existing, "style")
	if err != nil {
		t.Fatal(err)
	}
	if !changed || removeFile {
		t.Fatalf("remove result changed=%v removeFile=%v", changed, removeFile)
	}
	if bytes.Contains(updated, []byte("style --owner=style")) || !bytes.Contains(updated, []byte("other --owner=other")) {
		t.Errorf("owner removal did not preserve the sibling handler:\n%s", updated)
	}
}

func TestTOMLOwnerRegionsCanBeRemovedIndependently(t *testing.T) {
	styleManifest := testManifest()
	styleManifest.Owner = "style"
	styleFS, err := Render(styleManifest, Target{Provider: agenthooks.ProviderKimi, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	styleRegion := readRendered(t, styleFS, "config.toml")
	otherRegion := []byte(tomlBeginMarker + "\n[[hooks]]\nevent = \"Notification\"\n" + tomlEndMarker + "\n")
	existing := append([]byte("# keep this setting\n\n"), otherRegion...)
	merged := mergeManagedTOML(existing, styleRegion)
	removed, changed := removeManagedTOML(merged, "style")
	if !changed {
		t.Fatal("expected STYLE marker region to be removed")
	}
	if !bytes.Equal(removed, existing) {
		t.Fatalf("removing appended region changed external bytes:\n got %q\nwant %q", removed, existing)
	}
	if !bytes.Contains(removed, []byte("# keep this setting")) || !bytes.Contains(removed, []byte("event = \"Notification\"")) {
		t.Errorf("removing STYLE region damaged other TOML content:\n%s", removed)
	}
	styleBegin, _ := tomlMarkers("style")
	if bytes.Contains(removed, []byte(styleBegin)) {
		t.Errorf("STYLE marker remains after removal:\n%s", removed)
	}
}

func TestManagedTOMLRoundTripRestoresExternalBytes(t *testing.T) {
	styleManifest := testManifest()
	styleManifest.Owner = "style"
	styleFS, err := Render(styleManifest, Target{Provider: agenthooks.ProviderCodex, Scope: ScopeUser, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	rendered := readRendered(t, styleFS, "config.toml")
	for _, external := range [][]byte{
		[]byte("# external"),
		[]byte("# external\n"),
		[]byte("# external\n\n"),
	} {
		merged := mergeManagedTOML(external, rendered)
		removed, changed := removeManagedTOML(merged, "style")
		if !changed {
			t.Fatalf("expected owner region to be removed for %q", external)
		}
		if !bytes.Equal(removed, external) {
			t.Errorf("round-trip changed external TOML:\n got %q\nwant %q", removed, external)
		}
	}
}

// TestDefinitionHashCodexAlgorithm pins the exact identity serialization
// Codex hashes (verified against codex-cli 0.142.4 trust state): sha256 over
// compact, key-sorted JSON with HTML escaping disabled.
func TestDefinitionHashCodexAlgorithm(t *testing.T) {
	// Matcher present: {"event_name":"pre_tool_use","hooks":[{"async":false,
	// "command":"run <a> & \"b\"","timeout":30,"type":"command"}],"matcher":"Bash"}
	sum := sha256.Sum256([]byte(`{"event_name":"pre_tool_use","hooks":[{"async":false,"command":"run <a> & \"b\"","timeout":30,"type":"command"}],"matcher":"Bash"}`))
	if got, want := DefinitionHash("PreToolUse", "Bash", `run <a> & "b"`, 30), "sha256:"+hex.EncodeToString(sum[:]); got != want {
		t.Errorf("DefinitionHash = %s, want %s", got, want)
	}
	// Codex forces the matcher absent on Stop; absent timeout defaults to 600.
	sum = sha256.Sum256([]byte(`{"event_name":"stop","hooks":[{"async":false,"command":"cmd","timeout":600,"type":"command"}]}`))
	if got, want := DefinitionHash("Stop", "ignored", "cmd", 0), "sha256:"+hex.EncodeToString(sum[:]); got != want {
		t.Errorf("DefinitionHash(Stop) = %s, want %s", got, want)
	}
}

func TestInstallIdempotentAndMergePreservesForeignEntries(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeProject, Dir: dir, HarnessVersion: "2.1.300"}
	m := testManifest()

	// Pre-existing user config with a foreign hook and an unrelated key.
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := `{"env":{"FOO":"1"},"hooks":{"PreToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"other-tool check"}]}]}}`
	if err := os.WriteFile(settingsPath, []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	merged, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), "other-tool check") || !strings.Contains(string(merged), `"FOO"`) {
		t.Errorf("foreign config must survive merge:\n%s", merged)
	}
	if !bytes.Contains(merged, []byte(`"--provider=claude-code"`)) {
		t.Errorf("managed hooks missing after merge:\n%s", merged)
	}

	// Second install: nothing to do.
	changes, err := Diff(m, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.State != StateUnchanged {
			t.Errorf("expected idempotence, got %s for %s", c.State, c.Path)
		}
	}
}

func TestFingerprint(t *testing.T) {
	m := testManifest()
	target := Target{Provider: agenthooks.ProviderClaudeCode, Scope: ScopeProject, HarnessVersion: "2.1.300"}
	a, err := Fingerprint(m, target)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Fingerprint(m, target)
	if a != b {
		t.Error("fingerprint must be stable")
	}
	m.Hooks[0].Timeout = 45 * time.Second
	c, _ := Fingerprint(m, target)
	if a == c {
		t.Error("fingerprint must change with the manifest")
	}
}

func TestRenderKimiTOML(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderKimi, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	toml := string(readRendered(t, fsys, "config.toml"))
	if !strings.HasPrefix(toml, tomlBeginMarker) || !strings.Contains(toml, tomlEndMarker) {
		t.Errorf("managed markers missing:\n%s", toml)
	}
	if !strings.Contains(toml, `event = "PreToolUse"`) || !strings.Contains(toml, `matcher = "Bash"`) {
		t.Errorf("PreToolUse entry wrong:\n%s", toml)
	}
	if !strings.Contains(toml, "agenthooks run --provider=kimi-code") {
		t.Errorf("argv contract missing:\n%s", toml)
	}
	if !strings.Contains(toml, "timeout = 30") {
		t.Errorf("timeout must render in seconds:\n%s", toml)
	}

	// Kimi reads hooks from the single user-level config.toml only:
	// project scope must fail loudly, not render dead config.
	if _, err := Render(testManifest(), Target{Provider: agenthooks.ProviderKimi, Scope: ScopeProject}); err == nil {
		t.Error("project scope must be rejected for kimi")
	}
}

func TestInstallKimiMergePreservesForeignTOML(t *testing.T) {
	dir := t.TempDir()
	target := Target{Provider: agenthooks.ProviderKimi, Scope: ScopeUser, Dir: dir}
	m := testManifest()

	tomlPath := filepath.Join(dir, "config.toml")
	foreign := "[[hooks]]\nevent = \"Notification\"\ncommand = \"terminal-notifier -title Kimi\"\n"
	if err := os.WriteFile(tomlPath, []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	merged, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), "terminal-notifier") {
		t.Errorf("foreign hook must survive merge:\n%s", merged)
	}
	if !strings.Contains(string(merged), "agenthooks run --provider=kimi-code") {
		t.Errorf("managed hooks missing after merge:\n%s", merged)
	}

	// Second install: idempotent.
	changes, err := Diff(m, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.State != StateUnchanged {
			t.Errorf("expected idempotence, got %s for %s", c.State, c.Path)
		}
	}

	// Re-install after a manifest change replaces only the managed region.
	m.Hooks[0].Timeout = 45 * time.Second
	if err := Install(context.Background(), m, target); err != nil {
		t.Fatal(err)
	}
	merged, _ = os.ReadFile(tomlPath)
	if !strings.Contains(string(merged), "terminal-notifier") || !strings.Contains(string(merged), "timeout = 45") {
		t.Errorf("managed-region replacement broken:\n%s", merged)
	}
	if strings.Count(string(merged), tomlBeginMarker) != 1 {
		t.Errorf("markers must not duplicate:\n%s", merged)
	}
}

func TestRenderOpenCodeShim(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderOpenCode, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	shim := string(readRendered(t, fsys, ".opencode/plugin/agenthooks.ts"))
	if !strings.Contains(shim, `"/usr/local/bin/myhooks"`) {
		t.Errorf("shim must bake in the command:\n%s", shim[:200])
	}
	if !strings.Contains(shim, `"agenthooks", "serve", "--provider=opencode"`) {
		t.Error("shim must spawn serve mode")
	}
	for _, want := range []string{
		"ctx.client.config.get()", "?.data", "mcp === undefined",
		"type: server?.type", "command: server?.command", "url: server?.url", "enabled: server?.enabled",
		"await initialize()",
	} {
		if !strings.Contains(shim, want) {
			t.Errorf("shim must carry resolved MCP config via %q", want)
		}
	}
	for _, leak := range []string{"server?.headers", "server?.environment", "s?.headers", "s?.environment"} {
		if strings.Contains(shim, leak) {
			t.Errorf("shim must not forward MCP credentials (%s)", leak)
		}
	}
	for _, want := range []string{
		"export default { id: \"agenthooks\", setup, server: legacy }",
		"ctx.tool.hook(\"execute.before\"", "ctx.session.hook(\"prompt\"", "ctx.event.subscribe(",
		"ctx.mcp.transform(", "listed.every(", "own.has(sid)", "ev?.location?.directory === directory",
		"send(\"message.part.updated\"", "case \"session.execution.succeeded\":",
		"addUsage(usage.get(sid), d)", "case \"session.deleted\":",
		"ctx.session.get({ sessionID: ev.sessionID })", "announced.has(sid)", "return announce(sid, d.parentID)",
	} {
		if !strings.Contains(shim, want) {
			t.Errorf("shim must carry the OpenCode 2 entrypoint via %q", want)
		}
	}
	if strings.Contains(shim, "export const") {
		t.Error("a named export loads as a second OpenCode 1 plugin")
	}
}

// TestRenderCopilotPlugin pins the three things that silently break Copilot
// telemetry rather than failing loudly:
//   - a matcher key at all (an empty one is a validation error that discards
//     this plugin's ENTIRE hook config),
//   - a second copy of the config at <root>/hooks.json (Copilot parses both
//     paths, so shipping both double-registers every hook),
//   - bash/powershell keys (Copilot fills both from command; splitting them
//     here would render the argv twice with no test on the second copy).
func TestRenderCopilotPlugin(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderCopilotCLI, Scope: ScopePlugin})
	if err != nil {
		t.Fatal(err)
	}
	var plugin map[string]string
	if err := json.Unmarshal(readRendered(t, fsys, "plugin.json"), &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin["name"] != "myhooks" {
		t.Errorf("plugin.json must sit at the package root with the manifest name: %v", plugin)
	}
	if _, err := fs.ReadFile(fsys, "hooks.json"); err == nil {
		t.Error("hooks.json at the package root double-registers every hook")
	}

	raw := readRendered(t, fsys, "hooks/hooks.json")
	if bytes.Contains(raw, []byte(`"matcher"`)) {
		t.Errorf("matcher key present; an empty matcher discards the whole hook config:\n%s", raw)
	}
	var cfg struct {
		Version int `json:"version"`
		Hooks   map[string][]struct {
			Type       string          `json:"type"`
			Command    string          `json:"command"`
			TimeoutSec int             `json:"timeoutSec"`
			Bash       json.RawMessage `json:"bash"`
			PowerShell json.RawMessage `json:"powershell"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	// testManifest declares ToolPre, Stop and ToolPost; each maps to exactly one
	// native Copilot event, so there is no double-fire to dedupe.
	if len(cfg.Hooks) != 3 {
		t.Errorf("hooks = %v, want one entry per declared kind", cfg.Hooks)
	}
	pre := cfg.Hooks["preToolUse"]
	if len(pre) != 1 {
		t.Fatalf("preToolUse entries = %+v", pre)
	}
	if pre[0].Type != "command" || pre[0].TimeoutSec != 30 {
		t.Errorf("preToolUse entry wrong: %+v", pre[0])
	}
	if !strings.Contains(pre[0].Command, "agenthooks run --provider=copilot-cli") {
		t.Errorf("command wrong: %q", pre[0].Command)
	}
	if len(pre[0].Bash) > 0 || len(pre[0].PowerShell) > 0 {
		t.Error("bash/powershell must be absent; Copilot copies command into both")
	}
	for _, event := range []string{"agentStop", "postToolUse"} {
		if len(cfg.Hooks[event]) != 1 {
			t.Errorf("%s not registered: %v", event, cfg.Hooks)
		}
	}
}

func TestRenderCopilotScopes(t *testing.T) {
	// Project scope goes to .github/hooks/, user scope to hooks/hooks.json
	// under Target.Dir (~/.copilot). Plugin scope has nowhere to put the name.
	proj, err := Render(testManifest(), Target{Provider: agenthooks.ProviderCopilotCLI, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	readRendered(t, proj, ".github/hooks/agenthooks.json")

	user, err := Render(testManifest(), Target{Provider: agenthooks.ProviderCopilotCLI, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	readRendered(t, user, "hooks/hooks.json")

	m := testManifest()
	m.Identity.Name = ""
	if _, err := Render(m, Target{Provider: agenthooks.ProviderCopilotCLI, Scope: ScopePlugin}); err == nil {
		t.Error("plugin scope with no Identity.Name must fail, not emit a nameless package")
	}
}

// TestRenderVSCodeScopes pins the two paths and the basename. Both directories
// are globbed by VS Code AND by the Copilot CLI, so the basename is the only
// thing keeping this file from colliding with render_copilot.go's — and
// agenthooks-vscode.json is neither settings.json nor hooks.json, so the file
// stays whole-file owned instead of being merged into.
func TestRenderVSCodeScopes(t *testing.T) {
	proj, err := Render(testManifest(), Target{Provider: agenthooks.ProviderVSCodeCopilot, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	readRendered(t, proj, ".github/hooks/agenthooks-vscode.json")

	user, err := Render(testManifest(), Target{Provider: agenthooks.ProviderVSCodeCopilot, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	raw := readRendered(t, user, "hooks/agenthooks-vscode.json") // Target.Dir is ~/.copilot
	if isMergeableJSON("hooks/agenthooks-vscode.json") {
		t.Error("agenthooks-vscode.json must not be merge-eligible; a merge would fold it into the CLI's config")
	}

	if _, err := Render(testManifest(), Target{Provider: agenthooks.ProviderVSCodeCopilot, Scope: ScopePlugin}); err == nil {
		t.Error("plugin scope must fail: VS Code loads plugin hooks through ~/.copilot, which user scope already covers")
	}

	// PascalCase event keys, one per declared kind. A camelCase key here would
	// still resolve in VS Code (it accepts the CLI's names) but would pick up
	// the CLI's event vocabulary, where Stop is spelled agentStop.
	var cfg struct {
		Hooks map[string][]struct {
			Type       string `json:"type"`
			Command    string `json:"command"`
			Timeout    int    `json:"timeout"`
			TimeoutSec int    `json:"timeoutSec"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"PreToolUse", "Stop", "PostToolUse"} {
		if len(cfg.Hooks[event]) != 1 {
			t.Fatalf("%s not registered: %v", event, cfg.Hooks)
		}
	}
	if len(cfg.Hooks) != 3 {
		t.Errorf("hooks = %v, want one entry per declared kind", cfg.Hooks)
	}
	pre := cfg.Hooks["PreToolUse"][0]
	if pre.Type != "command" {
		t.Errorf("type = %q, want command", pre.Type)
	}
	if !strings.Contains(pre.Command, "agenthooks run --provider=vscode-copilot") {
		t.Errorf("command wrong: %q", pre.Command)
	}
	// VS Code parses matcher values and ignores them, so --filter is the only
	// enforcement that is actually true.
	if !strings.Contains(pre.Command, "--filter=") {
		t.Errorf("no --filter in %q; VS Code ignores matchers, so a scoped hook would fire on every tool", pre.Command)
	}
}

// TestRenderVSCodeOmissions pins the keys whose wrong presence or absence fails
// silently rather than loudly: a matcher that reads as enforcement VS Code does
// not perform, a version key no VS Code example carries (an unknown key is a
// schema-validation risk), bash/powershell keys VS Code does not understand at
// all (its platform override is windows), and the two unreconciled timeout
// spellings — the reference table says timeout, a usage example says
// timeoutSec, and reading the missing one silently means the 30s default.
func TestRenderVSCodeOmissions(t *testing.T) {
	fsys, err := Render(testManifest(), Target{Provider: agenthooks.ProviderVSCodeCopilot, Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	raw := readRendered(t, fsys, ".github/hooks/agenthooks-vscode.json")
	for _, key := range []string{`"matcher"`, `"version"`, `"bash"`, `"powershell"`, `"osx"`} {
		if bytes.Contains(raw, []byte(key)) {
			t.Errorf("%s key present:\n%s", key, raw)
		}
	}
	var cfg struct {
		Hooks map[string][]struct {
			Timeout    *int `json:"timeout"`
			TimeoutSec *int `json:"timeoutSec"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	pre := cfg.Hooks["PreToolUse"][0]
	if pre.Timeout == nil || pre.TimeoutSec == nil || *pre.Timeout != 30 || *pre.TimeoutSec != 30 {
		t.Errorf("both timeout spellings must carry the same value, got timeout=%v timeoutSec=%v", pre.Timeout, pre.TimeoutSec)
	}
	stop := cfg.Hooks["Stop"][0]
	if stop.Timeout == nil || *stop.Timeout != 60 {
		t.Errorf("Stop timeout = %v, want the 60s default", stop.Timeout)
	}
}

// TestHookCommandQuotesSpacedBinary pins the quoting of a consumer binary whose
// path contains spaces. The dialect follows the HOST OS, not the provider
// (shellQuote, install.go:355-374): configs are rendered on the machine that
// runs them, cmd.exe has no single-quote syntax and POSIX shells have no
// cmd-style escaping.
//
// The Windows form is cmd.exe's, and deliberately so: PowerShell parses a
// statement that begins with a quote as an expression, so it needs a leading
// call operator (& "C:\Program Files\...") that cmd.exe in turn rejects — no
// single string is valid in both. Both shells are reachable on Windows (the
// Copilot CLI copies command into its powershell key, render_copilot.go:34-36),
// so if a spaced path is ever observed failing under PowerShell the fix is a
// per-shell rendering for those providers, not a change to this quoting.
func TestHookCommandQuotesSpacedBinary(t *testing.T) {
	m := testManifest()
	m.Command = []string{"/opt/My Hooks/myhooks"}
	want := `'/opt/My Hooks/myhooks' agenthooks run --provider=vscode-copilot`
	if runtime.GOOS == "windows" {
		want = `"/opt/My Hooks/myhooks" agenthooks run --provider=vscode-copilot`
	}
	got := hookCommand(m, agenthooks.ProviderVSCodeCopilot, m.Hooks[0])
	if !strings.HasPrefix(got, want) {
		t.Errorf("spaced binary path unquoted or wrong dialect:\n got %s\nwant prefix %s", got, want)
	}
}
