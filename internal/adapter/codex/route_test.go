package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestCodexSurfaceAndManifestStayFixed(t *testing.T) {
	surface := Surface()
	want := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart"}
	if len(surface) != len(want) {
		t.Fatalf("surface has %d hooks, want %d: %#v", len(surface), len(want), surface)
	}
	for i, event := range want {
		if surface[i].NativeEvent != event {
			t.Errorf("surface[%d] = %q, want %q", i, surface[i].NativeEvent, event)
		}
	}
	manifest := HookManifest([]string{"/tmp/style"})
	if manifest.Owner != Owner || len(manifest.Hooks) != len(want) {
		t.Fatalf("manifest owner/hooks = %q/%d", manifest.Owner, len(manifest.Hooks))
	}
	if manifest.Hooks[2].NativeMatcher != `^(Bash|apply_patch|mcp__.*)$` || manifest.Hooks[3].NativeMatcher != manifest.Hooks[2].NativeMatcher {
		t.Fatalf("tool hooks lost native matcher: %#v", manifest.Hooks[2:4])
	}
	for _, hook := range manifest.Hooks {
		if !hook.Blocking || hook.Timeout <= 0 {
			t.Fatalf("fixed Codex hook must be a bounded blocking hook: %#v", hook)
		}
	}
}

func TestCodexFactsDetectUnavailableStaticSlotAndUnobservableTools(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("developer_instructions = \"user owned\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	definition := DefinitionForInstallation(adapter.Installation{Provider: ProviderName, ConfigDir: home})
	if location, ok := definition.Facts.StaticLocation("append"); ok || location.Supported {
		t.Fatalf("user-owned static slot is available: %#v, %v", location, ok)
	}
	if len(definition.StaticPositions.StaticPositions(adapter.Installation{Provider: ProviderName, ConfigDir: home})) != 0 {
		t.Fatal("static position catalog included an unavailable user-owned slot")
	}

	tests := []struct {
		name  string
		tool  any
		names any
		want  bool
	}{
		{name: "unobserved native Edit", names: "Edit", want: true},
		{name: "observed native Bash", names: "Bash", want: false},
		{name: "observed apply_patch", names: []any{"apply_patch"}, want: false},
		{name: "unobserved file read class", tool: "file.read", want: true},
		{name: "observed shell class", tool: "shell", want: false},
		{name: "MCP target", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := spec.Rule{Effective: map[string]any{"target": "tool"}}
			if test.name == "MCP target" {
				rule.Effective["target"] = "mcp"
			} else {
				if test.tool != nil {
					rule.Effective["tool"] = test.tool
				}
				if test.names != nil {
					rule.Effective["names"] = test.names
				}
			}
			_, got := definition.Facts.BlindSpot(rule)
			if got != test.want {
				t.Fatalf("BlindSpot() = %v, want %v", got, test.want)
			}
		})
	}
	if !definition.Facts.Capability(plan.SubagentStart, plan.AddContext).Available {
		t.Fatal("Codex subagent.start must support add-context")
	}
	if available := definition.Facts.DataSource(plan.ToolPost, "skill.name"); !available.Available {
		t.Fatalf("implicit skill.name should be available on tool.post: %#v", available)
	}
}

func TestCodexPluginStatusDoesNotClaimTrustBeforeConfirmation(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	route := NewPluginRouteForCommand(filepath.Join(root, "plugin"), codexHome, "/tmp/style", nil)
	status, err := route.Status(context.Background(), adapter.Installation{Provider: ProviderName, ConfigDir: filepath.Join(root, "codex-home")})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || status.Installed || status.Trusted {
		t.Fatalf("uninstalled plugin status = %#v", status)
	}
	changes, err := route.Plan(context.Background(), Surface(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := route.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	status, err = route.Status(context.Background(), adapter.Installation{Provider: ProviderName, ConfigDir: codexHome})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || status.Trusted {
		t.Fatalf("installed-but-unconfirmed plugin status = %#v", status)
	}
}

func TestCodexPluginRoutePlansStaticAndHookConfigAtomically(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugin")
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	external := []byte("# user bytes before STYLE\nmodel = \"gpt-5\"\n\n[features]\nfoo = true\n")
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.WriteFile(configPath, external, 0o640); err != nil {
		t.Fatal(err)
	}
	facts := NewFacts(adapter.Installation{Provider: ProviderName, ConfigDir: codexHome})
	route := NewPluginRouteForCommand(pluginDir, codexHome, "/tmp/style binary", &facts)
	blocks := []adapter.StaticBlock{{Owner: "pkg-a", Slot: StaticSlotAppend, Body: "Follow package A."}}
	changes, err := route.Plan(context.Background(), Surface(), blocks)
	if err != nil {
		t.Fatal(err)
	}
	configChange := findChange(changes, configPath)
	if configChange == nil {
		t.Fatalf("plan did not include shared config.toml: %#v", changes)
	}
	if configChange.CreateOnly || configChange.Before == nil || string(configChange.Before) != string(external) {
		t.Fatalf("config change lost existing snapshot/create semantics: %#v", configChange)
	}
	if !strings.HasPrefix(string(configChange.After), staticBegin+"\n") || strings.Index(string(configChange.After), "developer_instructions") > strings.Index(string(configChange.After), "[features]") {
		t.Fatalf("developer_instructions is not a root key before tables:\n%s", configChange.After)
	}
	if !strings.Contains(string(configChange.After), "[hooks.state.") || !strings.Contains(string(configChange.After), "Follow package A.") {
		t.Fatalf("combined config lacks managed hooks or static instructions:\n%s", configChange.After)
	}
	if err := route.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	withoutStatic, err := removeStaticTOML(installed, Owner)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withoutStatic), string(external)) {
		t.Fatalf("external TOML bytes were not preserved:\n%s", withoutStatic)
	}

	second, err := route.Plan(context.Background(), Surface(), blocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Files) != 0 {
		t.Fatalf("repeated plan should be idempotent, got %#v", second.Files)
	}

	updatedBlocks := []adapter.StaticBlock{{Owner: "pkg-a", Slot: StaticSlotAppend, Body: "Updated instruction."}}
	staticOnly, err := route.Plan(context.Background(), Surface(), updatedBlocks)
	if err != nil {
		t.Fatal(err)
	}
	staticChange := findChange(staticOnly, configPath)
	if staticChange == nil || !strings.Contains(string(staticChange.After), "Updated instruction.") || strings.Contains(string(staticChange.After), "Follow package A.") {
		t.Fatalf("static-only plan did not update developer instructions: %#v", staticOnly)
	}
	if err := route.Apply(context.Background(), staticOnly); err != nil {
		t.Fatal(err)
	}

	staleBlocks := []adapter.StaticBlock{{Owner: "pkg-a", Slot: StaticSlotAppend, Body: "Stale planned instruction."}}
	stale, err := route.Plan(context.Background(), Surface(), staleBlocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale.Files) == 0 {
		t.Fatal("expected plugin configuration to be installed before stale test")
	}
	if err := os.WriteFile(configPath, append(installed, []byte("# changed after plan\n")...), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := route.Apply(context.Background(), stale); err == nil || !strings.Contains(err.Error(), "changed before apply") {
		t.Fatalf("stale plan error = %v", err)
	}
}

func TestCodexRouteRemoveCleansStaticOnlyAndKeepsExternalBytes(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	external := []byte("model = \"gpt-5\"\n\n[features]\nfoo = true\n")
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.WriteFile(configPath, external, 0o600); err != nil {
		t.Fatal(err)
	}
	facts := NewFacts(adapter.Installation{Provider: ProviderName, ConfigDir: codexHome})
	route := NewUserRouteForCommand(codexHome, "/tmp/style", &facts)
	blocks := []adapter.StaticBlock{{Owner: "pkg-a", Slot: StaticSlotAppend, Body: "Only static."}}
	changes, err := route.Plan(context.Background(), Surface(), blocks)
	if err != nil {
		t.Fatal(err)
	}
	if err := route.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	// Simulate the hook trust block being removed externally while the STYLE
	// static region remains. Route removal must still remove that region.
	if err := os.WriteFile(configPath, append([]byte(staticBegin+"\n"+`developer_instructions = "leftover"`+"\n"+staticEnd+"\n"), external...), 0o600); err != nil {
		t.Fatal(err)
	}

	removePlan, err := route.Remove(context.Background(), adapter.Owner(Owner))
	if err != nil {
		t.Fatal(err)
	}
	if findChange(removePlan, configPath) == nil {
		t.Fatalf("remove did not include static-only config change: %#v", removePlan)
	}
	if err := route.Apply(context.Background(), removePlan); err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != string(external) {
		t.Fatalf("remove did not preserve exact external bytes:\n got: %q\nwant: %q", result, external)
	}
}

func TestCodexPluginApplyUsesNarrowestRoot(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	pluginDir := filepath.Join(codexHome, "nested", "plugin")
	facts := DefaultFacts()
	routeValue := NewPluginRouteForCommand(pluginDir, codexHome, "/tmp/style", &facts)
	changes, err := routeValue.Plan(context.Background(), Surface(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := routeValue.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	for _, change := range changes.Files {
		if filepath.Dir(change.Path) == pluginDir || strings.HasPrefix(change.Path, pluginDir+string(filepath.Separator)) {
			root, relative, err := routeValue.(*route).splitDestination(change.Path)
			if err != nil || root != pluginDir || strings.HasPrefix(relative, "..") {
				t.Fatalf("plugin path root = %q/%q, err=%v", root, relative, err)
			}
		}
	}
}

func findChange(changes adapter.Changeset, path string) *adapter.FileChange {
	for i := range changes.Files {
		if filepath.Clean(changes.Files[i].Path) == filepath.Clean(path) {
			return &changes.Files[i]
		}
	}
	return nil
}

func TestCodexUserRouteWritesAbsoluteHookCommandForRelativeProgram(t *testing.T) {
	saved := os.Args[0]
	defer func() { os.Args[0] = saved }()
	os.Args[0] = "./style"
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	changes, err := NewUserRouteForCommand(dir, "", nil).(*route).BindStorage(filepath.Join(dir, "style-home")).Plan(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hooks := ""
	for _, change := range changes.Files {
		if filepath.Base(change.Path) == "hooks.json" {
			hooks = string(change.After)
		}
	}
	want := filepath.Join(cwd, "style") + " --style-home="
	if !strings.Contains(hooks, want) || strings.Contains(hooks, `"./style`) {
		t.Fatalf("hooks.json command must use %q, got:\n%s", want, hooks)
	}
}
