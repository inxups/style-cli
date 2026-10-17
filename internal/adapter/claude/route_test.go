package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/internal/adapter"
)

func findChange(changes adapter.Changeset, path string) *adapter.FileChange {
	for i := range changes.Files {
		if filepath.Clean(changes.Files[i].Path) == filepath.Clean(path) {
			return &changes.Files[i]
		}
	}
	return nil
}

func TestClaudeRoutePlansDynamicSettingsAndStaticBlocks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	staticPath := filepath.Join(dir, "CLAUDE.md")
	existingSettings := []byte(`{"env":{"KEEP":"yes"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"other --owner=other"}]}]}}`)
	if err := os.WriteFile(settingsPath, existingSettings, 0o640); err != nil {
		t.Fatal(err)
	}
	existingStatic := []byte("# User instructions\n\n<!-- style:other -->\nOther package\n<!-- /style:other -->\n")
	if err := os.WriteFile(staticPath, existingStatic, 0o600); err != nil {
		t.Fatal(err)
	}
	facts := NewFacts(adapter.Installation{Provider: ProviderName, ConfigDir: dir})
	route := NewUserRouteForInstallation(dir, "2.1.315", "/tmp/style", &facts)
	hooks := []adapter.HookSpec{
		{NativeEvent: "PreToolUse", Matcher: `^(Edit|Write)$`, Points: []string{plan.ToolPre}},
		{NativeEvent: "PostModelSwitch", Points: []string{"model.changed"}},
	}
	blocks := []adapter.StaticBlock{{Owner: "pkg-a", Slot: StaticSlotAppend, Body: "Review every change."}}
	changes, err := route.Plan(context.Background(), hooks, blocks)
	if err != nil {
		t.Fatal(err)
	}
	settingsChange := findChange(changes, settingsPath)
	if settingsChange == nil || settingsChange.Before == nil || !strings.Contains(string(settingsChange.After), "other --owner=other") || !strings.Contains(string(settingsChange.After), "PostModelSwitch") {
		t.Fatalf("settings plan did not preserve foreign hook or native event: %#v", changes)
	}
	staticChange := findChange(changes, filepath.Join(dir, "rules", "style-pkg-a.md"))
	if findChange(changes, staticPath) != nil || staticChange == nil || !strings.Contains(string(staticChange.After), "Review every change.") {
		t.Fatal("static plan must create a package rules file and preserve foreign CLAUDE.md")
	}
	if err := route.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	status, err := route.Status(context.Background(), adapter.Installation{Provider: ProviderName, ConfigDir: dir})
	if err != nil || !status.Available || !status.Installed || status.Trusted {
		t.Fatalf("status after install = %#v, err=%v", status, err)
	}
	second, err := route.Plan(context.Background(), hooks, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Files) != 0 {
		t.Fatalf("repeated plan is not idempotent: %#v", second.Files)
	}
}

func TestClaudeRouteRemovePreservesForeignSettingsAndStaticOwners(t *testing.T) {
	dir := t.TempDir()
	route := NewUserRouteForInstallation(dir, "2.1.315", "/tmp/style", nil)
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"foreign":"keep"},"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"other --owner=other"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	hooks := []adapter.HookSpec{{NativeEvent: "PreToolUse", Matcher: "Edit", Points: []string{plan.ToolPre}}}
	blocks := []adapter.StaticBlock{{Owner: "pkg-a", Body: "A"}, {Owner: "pkg-b", Body: "B"}}
	changes, err := route.Plan(context.Background(), hooks, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if err := route.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	remove, err := route.Remove(context.Background(), adapter.Owner(Owner))
	if err != nil {
		t.Fatal(err)
	}
	if findChange(remove, settingsPath) == nil || findChange(remove, filepath.Join(dir, "rules", "style-pkg-a.md")) == nil || findChange(remove, filepath.Join(dir, "rules", "style-pkg-b.md")) == nil {
		t.Fatal("remove plan omitted owner files")
	}
	if err := route.Apply(context.Background(), remove); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(settingsPath); err != nil || strings.Contains(string(got), "--owner=style") || !strings.Contains(string(got), "foreign") {
		t.Fatalf("settings owner removal = %q, err=%v", got, err)
	}
	for _, name := range []string{"style-pkg-a.md", "style-pkg-b.md"} {
		if _, err := os.Stat(filepath.Join(dir, "rules", name)); !os.IsNotExist(err) {
			t.Fatalf("owned rules file should be removed, err=%v", err)
		}
	}
}

func TestClaudeRouteRemoveKeepsUserBytesAroundStaticBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	before := []byte("before\n\n<!-- style:pkg-a -->\nA\n<!-- /style:pkg-a -->\n\nafter\n")
	if err := os.WriteFile(path, before, 0o640); err != nil {
		t.Fatal(err)
	}
	route := NewUserRoute(dir, nil)
	changes, err := route.Remove(context.Background(), adapter.Owner("pkg-a"))
	if err != nil {
		t.Fatal(err)
	}
	if err := route.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before\n\nafter\n" {
		t.Fatalf("user bytes after static owner removal = %q", got)
	}
}

func TestClaudeRouteRejectsMalformedStaticMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("<!-- style:pkg-a -->\nmissing end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	route := NewUserRoute(dir, nil)
	if _, err := route.Plan(context.Background(), nil, []adapter.StaticBlock{{Owner: "pkg-b", Body: "B"}}); err == nil {
		t.Fatal("malformed static marker was accepted")
	}
}

func TestClaudeRouteWritesAbsoluteHookCommandForRelativeProgram(t *testing.T) {
	saved := os.Args[0]
	defer func() { os.Args[0] = saved }()
	os.Args[0] = "./style"
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	hooks := []adapter.HookSpec{{NativeEvent: "PreToolUse", Matcher: `^Bash$`, Points: []string{plan.ToolPre}}}
	changes, err := NewUserRouteForInstallation(dir, "2.1.315", "", nil).(*route).BindStorage(filepath.Join(dir, "style-home")).Plan(context.Background(), hooks, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings := findChange(changes, filepath.Join(dir, "settings.json"))
	if settings == nil {
		t.Fatalf("settings.json not planned: %#v", changes.Files)
	}
	quoted, _ := json.Marshal(filepath.Join(cwd, "style"))
	want := string(quoted)
	if !strings.Contains(string(settings.After), want) || strings.Contains(string(settings.After), `"./style`) {
		t.Fatalf("settings.json command must use %q, got:\n%s", want, settings.After)
	}
}
