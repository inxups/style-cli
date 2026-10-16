package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

const (
	ProviderName       = agenthooks.ProviderCodex
	Owner              = "style"
	PluginRouteName    = "codex-plugin"
	UserRouteName      = "codex-user"
	StaticSlotAppend   = "developer_instructions"
	DefaultInputTokens = 2500
)

// Surface returns the fixed Codex hook subscription surface. Rules are
// filtered in-process after decoding, so installing another STYLE package does
// not change Codex's trust fingerprint.
func Surface() []adapter.HookSpec {
	return []adapter.HookSpec{
		{NativeEvent: "SessionStart", Points: []string{plan.SessionStart}},
		{NativeEvent: "UserPromptSubmit", Points: []string{plan.PromptSubmitted, plan.SkillExplicit}},
		{NativeEvent: "PreToolUse", Matcher: `^(Bash|apply_patch|mcp__.*)$`, Points: []string{plan.ToolPre}},
		{NativeEvent: "PostToolUse", Matcher: `^(Bash|apply_patch|mcp__.*)$`, Points: []string{plan.ToolPost}},
		{NativeEvent: "SubagentStart", Points: []string{plan.SubagentStart}},
	}
}

// Facts implements the provider facts consumed by core/plan.
type Facts struct {
	StaticAppendAvailable bool
}

// NewFacts returns facts for an installation. A user-owned root
// developer_instructions key disables the static slot; callers can then let
// core/plan choose the dynamic fallback.
func NewFacts(installation adapter.Installation) Facts {
	available := true
	if installation.ConfigDir != "" {
		data, err := os.ReadFile(filepath.Join(installation.ConfigDir, "config.toml"))
		if err == nil {
			available = staticSlotAvailable(data)
		} else if !errors.Is(err, os.ErrNotExist) {
			available = false
		}
	}
	return Facts{StaticAppendAvailable: available}
}

func DefaultFacts() Facts { return Facts{StaticAppendAvailable: true} }

func (f Facts) Capability(point string, capability plan.Capability) adapter.CapabilitySupport {
	available := map[string]map[plan.Capability]bool{
		plan.SessionStart:    {plan.AddContext: true},
		plan.PromptSubmitted: {plan.AddContext: true, plan.SystemMessage: true, plan.Deny: true},
		plan.SkillExplicit:   {plan.AddContext: true, plan.SystemMessage: true, plan.Deny: true},
		plan.ToolPre:         {plan.AddContext: true, plan.SystemMessage: true, plan.Deny: true},
		plan.ToolPost:        {plan.AddContext: true, plan.SystemMessage: true},
		plan.SubagentStart:   {plan.AddContext: true},
	}
	if available[point][capability] {
		return adapter.CapabilitySupport{Available: true, Reason: "Codex hook codec supports this output"}
	}
	return adapter.CapabilitySupport{Reason: "Codex has no output capability at this evaluation point"}
}

func (f Facts) DataSource(point, field string) adapter.DataAvailability {
	common := map[string]bool{
		"model": true, "thinking_effort": true, "thinking_effort_raw": true,
		"thinking_effort_approx": true, "harness.name": true, "harness.version": true,
		"harness.cwd": true, "harness.permission_mode": true, "harness.session_id": true,
		"harness.turn_id": true, "harness.agent": true, "agent": true,
	}
	if common[field] {
		return adapter.DataAvailability{Available: true, Source: "Codex event/session state"}
	}
	switch point {
	case plan.PromptSubmitted, plan.SkillExplicit:
		if field == "prompt" || field == "skill.name" {
			return adapter.DataAvailability{Available: true, Source: "Codex UserPromptSubmit payload"}
		}
	case plan.ToolPre:
		if toolFieldAvailable(field, false) {
			return adapter.DataAvailability{Available: true, Source: "Codex PreToolUse payload"}
		}
	case plan.ToolPost:
		if toolFieldAvailable(field, true) {
			return adapter.DataAvailability{Available: true, Source: "Codex PostToolUse payload"}
		}
	case plan.SubagentStart:
		if field == "agent" || field == "harness.agent" {
			return adapter.DataAvailability{Available: true, Source: "Codex SubagentStart payload"}
		}
	}
	return adapter.DataAvailability{Reason: "Codex does not expose this field at the evaluation point"}
}

func toolFieldAvailable(field string, post bool) bool {
	if field == "tool.name" || field == "tool.canonical" || field == "tool.input" ||
		field == "tool.added" || field == "mcp.server" || field == "mcp.tool" || field == "skill.name" || field == "cwd" {
		return true
	}
	if post && field == "tool.output" {
		return true
	}
	return strings.HasPrefix(field, "tool.input.")
}

func (f Facts) BlindSpot(rule spec.Rule) (string, bool) {
	target, _ := rule.Effective["target"].(string)
	if target == "skill" {
		return "Codex skill hooks observe explicit $name and local SKILL.md reads; cloud skill reads are unsupported by this route", false
	}
	if target != "tool" && target != "mcp" {
		return "", false
	}
	if target == "mcp" {
		return "", false
	}
	toolSelectors, _ := stringSelectors(rule.Effective["tool"])
	nameSelectors, _ := stringSelectors(rule.Effective["names"])
	if len(toolSelectors)+len(nameSelectors) == 0 {
		return "", false
	}
	for _, selector := range toolSelectors {
		if codexCanonicalToolObservable(selector) {
			return "", false
		}
	}
	for _, selector := range nameSelectors {
		if codexNativeNameObservable(selector) {
			return "", false
		}
	}
	return "Codex fixed subscription does not observe these tool selectors", true
}

func codexCanonicalToolObservable(selector string) bool {
	switch strings.ToLower(selector) {
	case "*", "shell", "file.edit", "mcp":
		return true
	}
	return false
}

func codexNativeNameObservable(selector string) bool {
	return selector == "Bash" || selector == "apply_patch" || strings.HasPrefix(selector, "mcp__")
}

func stringSelectors(value any) ([]string, bool) {
	switch selectors := value.(type) {
	case string:
		return []string{selectors}, true
	case []string:
		return selectors, true
	case []any:
		values := make([]string, 0, len(selectors))
		for _, selector := range selectors {
			text, ok := selector.(string)
			if !ok {
				return nil, false
			}
			values = append(values, text)
		}
		return values, true
	default:
		return nil, false
	}
}

func (f Facts) StaticLocation(mode string) (adapter.StaticLocation, bool) {
	if mode != "append" || !f.StaticAppendAvailable {
		return adapter.StaticLocation{Mode: mode, Reason: "developer_instructions is already user-owned"}, false
	}
	return adapter.StaticLocation{Name: StaticSlotAppend, Mode: mode, Supported: true}, true
}

func (f Facts) StaticPositions(installation adapter.Installation) []adapter.StaticLocation {
	if installation.ConfigDir != "" {
		f = NewFacts(installation)
	}
	location, ok := f.StaticLocation("append")
	if !ok {
		return nil
	}
	return []adapter.StaticLocation{location}
}

// Detector discovers Codex without writing configuration.
type Detector struct {
	Home     string
	LookPath func(string) (string, error)
	Run      func(context.Context, string, ...string) ([]byte, error)
}

func (d Detector) Detect(ctx context.Context) (adapter.Installation, bool, error) {
	home := d.Home
	if home == "" {
		home = os.Getenv("CODEX_HOME")
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return adapter.Installation{}, false, nil
		}
		home = filepath.Join(userHome, ".codex")
	}
	lookPath := d.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	run := d.Run
	if run == nil {
		run = func(ctx context.Context, path string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, path, args...).Output()
		}
	}
	path, pathErr := lookPath("codex")
	_, configErr := os.Stat(filepath.Join(home, "config.toml"))
	if pathErr != nil && configErr != nil {
		return adapter.Installation{}, false, nil
	}
	version := ""
	if pathErr == nil {
		output, err := run(ctx, path, "--version")
		if err == nil {
			version = parseVersion(string(output))
		}
	}
	return adapter.Installation{Provider: ProviderName, Version: version, ConfigDir: home}, true, nil
}

var versionPattern = regexp.MustCompile(`(?m)(?:^|\s)v?(\d+\.\d+\.\d+)(?:\s|$)`)

func parseVersion(value string) string {
	match := versionPattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return match[1]
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[0], "v")
}

// Definition returns the offline Codex adapter definition. The default
// routes are ordered plugin first, then user-level hooks.json.
func Definition() adapter.Definition {
	installation := adapter.Installation{Provider: ProviderName, ConfigDir: defaultCodexHome()}
	return DefinitionForInstallation(installation)
}

// DefinitionForInstallation binds static-position facts and routes to one
// detected Codex home without mutating its configuration.
func DefinitionForInstallation(installation adapter.Installation) adapter.Definition {
	if installation.ConfigDir == "" {
		installation.ConfigDir = defaultCodexHome()
	}
	installation.Provider = ProviderName
	facts := NewFacts(installation)
	return adapter.Definition{
		ID:              ProviderName,
		Detector:        Detector{},
		Facts:           facts,
		StaticPositions: facts,
		Routes:          []adapter.Route{NewPluginRouteForCommand("", installation.ConfigDir, "", &facts), NewUserRouteForCommand(installation.ConfigDir, "", &facts)},
		Limits:          adapter.Limits{Input: tokens.InputLimit{Value: DefaultInputTokens, Unit: tokens.TokensUnit}, Approximate: true, Source: "Codex additionalContext default", Overflow: adapter.OverflowFilePreview},
		Compact:         adapter.CompactSignal{Event: agenthooks.KindSessionStart, SessionSource: "compact", Deferred: true},
		Native:          NativeSurface{},
	}
}

// NewDefinition is an alias suitable for registry construction.
func NewDefinition() adapter.Definition { return Definition() }

// HookManifest translates the fixed surface into the shared installation
// manifest. It deliberately keeps hook behavior in agenthooks/install.
func HookManifest(command []string, requests ...[]adapter.HookSpec) install.Manifest {
	hooks := make([]install.HookSpec, 0, len(Surface()))
	for _, surface := range Surface() {
		kind := nativeKind(surface.NativeEvent)
		spec := install.HookSpec{Kind: kind, Blocking: true, Timeout: 10 * time.Second, NativeMatcher: surface.Matcher}
		if surface.NativeEvent == "PreToolUse" || surface.NativeEvent == "PostToolUse" {
			spec.Tools = agenthooks.ToolMatcher{Names: []string{"Bash", "apply_patch"}, MCP: []string{"*/*"}}
		}
		hooks = append(hooks, spec)
	}
	if len(requests) > 0 {
		for _, hook := range requests[0] {
			if hook.Command == "" {
				continue
			}
			seconds, _ := hook.Extra["timeout_seconds"].(int)
			hooks = append(hooks, install.HookSpec{NativeEvent: hook.NativeEvent, NativeMatcher: hook.Matcher,
				Blocking: true, Timeout: time.Duration(seconds) * time.Second, RawCommand: hook.Command, RawTag: hook.NativeTag})
		}
	}
	return install.Manifest{Command: append([]string(nil), command...), Hooks: hooks, Identity: install.Identity{Name: "style-runtime", Version: "0.1.0", Description: "STYLE runtime hooks", PluginID: "style-runtime@style"}, Owner: Owner}
}

func nativeKind(native string) agenthooks.EventKind {
	for _, candidate := range []struct {
		name string
		kind agenthooks.EventKind
	}{
		{"SessionStart", agenthooks.KindSessionStart}, {"UserPromptSubmit", agenthooks.KindPromptSubmitted},
		{"PreToolUse", agenthooks.KindToolPre}, {"PostToolUse", agenthooks.KindToolPost}, {"SubagentStart", agenthooks.KindSubagentStart},
	} {
		if candidate.name == native {
			return candidate.kind
		}
	}
	return agenthooks.KindOther
}

func NewPluginRoute(dir, configDir string, facts *Facts) adapter.Route {
	return NewPluginRouteForCommand(dir, configDir, "", facts)
}

func NewUserRoute(dir string, facts *Facts) adapter.Route {
	return NewUserRouteForCommand(dir, "", facts)
}

var _ adapter.PlanningFacts = Facts{}
var _ adapter.StaticPositionProvider = Facts{}
var _ adapter.Detector = Detector{}
