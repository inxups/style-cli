package claude

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestSurfaceMergesAndOrdersDynamicClaudeHooks(t *testing.T) {
	input := []adapter.HookSpec{
		{NativeEvent: "PreToolUse", Matcher: "Edit", Points: []string{plan.ToolPre}},
		{NativeEvent: "PostModelSwitch", Points: []string{"model.changed"}},
		{NativeEvent: "PreToolUse", Matcher: "Edit", Points: []string{plan.ToolPre, "custom.point"}},
		{NativeEvent: "PostToolUseFailure", Matcher: "Bash", Points: []string{plan.ToolError}},
		{NativeEvent: "SessionStart", Points: []string{plan.SessionStart}},
		{NativeEvent: "UserPromptExpansion", Points: []string{"", plan.SkillExplicit}},
		{NativeEvent: "", Points: []string{plan.ToolPost}},
		{NativeEvent: "PostToolUse", Points: []string{" "}},
	}
	got := Surface(input)
	var native []string
	for _, hook := range got {
		native = append(native, hook.NativeEvent)
	}
	wantNative := []string{"SessionStart", "UserPromptExpansion", "PreToolUse", "PostToolUseFailure", "PostModelSwitch"}
	if !reflect.DeepEqual(native, wantNative) {
		t.Fatalf("normalized event order = %#v, want %#v", native, wantNative)
	}
	if len(got[2].Points) != 2 || got[2].Points[0] != plan.ToolPre || got[2].Points[1] != "custom.point" {
		t.Fatalf("duplicate event/matcher was not merged: %#v", got[2])
	}
	if got[2].Matcher != "Edit" || got[3].Matcher != "Bash" {
		t.Fatalf("native matchers lost: %#v", got)
	}
	for _, hook := range got {
		if hook.NativeEvent == "PostModelSwitch" && !reflect.DeepEqual(hook.Points, []string{"model.changed"}) {
			t.Fatalf("native-only event dropped: %#v", hook)
		}
	}
}

func TestClaudeFactsReportCapabilitiesDataAndStaticSlot(t *testing.T) {
	facts := DefaultFacts()
	for _, point := range []string{plan.SessionStart, plan.PromptSubmitted, plan.SkillExplicit, plan.ToolPre, plan.ToolPost, plan.ToolError} {
		if !facts.Capability(point, plan.AddContext).Available {
			t.Errorf("Claude add-context unavailable at %s", point)
		}
	}
	if facts.Capability(plan.SubagentStart, plan.AddContext).Available {
		t.Fatal("Claude subagent.start must not claim output support")
	}
	if !facts.Capability(plan.ToolPre, plan.Deny).Available || facts.Capability(plan.ToolPost, plan.Deny).Available {
		t.Fatal("deny capability does not match Claude's tool lifecycle")
	}
	for _, test := range []struct {
		point, field string
		want         bool
	}{
		{plan.PromptSubmitted, "prompt", true},
		{plan.SkillExplicit, "skill.name", true},
		{plan.ToolPre, "tool.input.file_path", true},
		{plan.ToolPre, "tool.output", false},
		{plan.ToolPost, "tool.output", true},
		{plan.ToolError, "tool.output", true},
		{plan.ToolPost, "tool.added", true},
	} {
		if got := facts.DataSource(test.point, test.field).Available; got != test.want {
			t.Errorf("DataSource(%q, %q).Available = %v, want %v", test.point, test.field, got, test.want)
		}
	}
	if location, ok := facts.StaticLocation("append"); !ok || !location.Supported || location.Name != StaticSlotAppend {
		t.Fatalf("Claude append slot = %#v, %v", location, ok)
	}
	if _, ok := facts.StaticLocation("replace"); ok {
		t.Fatal("Claude unexpectedly supports replace")
	}
	if _, blind := facts.BlindSpot(spec.Rule{Effective: map[string]any{"target": "tool", "names": "Edit"}}); blind {
		t.Fatal("Claude native matcher should observe arbitrary tool names")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("<!-- style:broken -->\nmissing end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unavailable := NewFacts(adapter.Installation{Provider: ProviderName, ConfigDir: dir})
	if location, ok := unavailable.StaticLocation("append"); ok || location.Supported {
		t.Fatalf("malformed managed region reported available: %#v, %v", location, ok)
	}
}

func TestClaudeDetectorUsesConfigOverrideAndReportsVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var lookName, executable string
	var args []string
	detector := Detector{
		Home: dir,
		LookPath: func(name string) (string, error) {
			lookName = name
			return "/fake/claude", nil
		},
		Run: func(_ context.Context, path string, commandArgs ...string) ([]byte, error) {
			executable, args = path, commandArgs
			return []byte("2.1.315 (Claude Code)\n"), nil
		},
	}
	installation, installed, err := detector.Detect(context.Background())
	if err != nil || !installed {
		t.Fatalf("Detect() = %#v, %v, %v", installation, installed, err)
	}
	if installation.Provider != ProviderName || installation.ConfigDir != dir || installation.Version != "2.1.315" {
		t.Fatalf("installation = %#v", installation)
	}
	if lookName != "claude" || executable != "/fake/claude" || !reflect.DeepEqual(args, []string{"--version"}) {
		t.Fatalf("detector invocation = %q %q, LookPath(%q)", executable, args, lookName)
	}
	missing := Detector{Home: t.TempDir(), LookPath: func(string) (string, error) { return "", os.ErrNotExist }}
	if _, installed, err := missing.Detect(context.Background()); err != nil || installed {
		t.Fatalf("missing Claude installation = installed %v, err %v", installed, err)
	}
}
