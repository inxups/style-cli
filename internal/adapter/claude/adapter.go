package claude

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	ProviderName      = agenthooks.ProviderClaudeCode
	Owner             = "style"
	UserRouteName     = "claude-user"
	StaticSlotAppend  = "claude_md"
	DefaultInputChars = 10000
)

// Surface normalizes a dynamic Claude hook subscription. Claude lets us attach
// a matcher to every tool event, so the adapter does not need a fixed native
// tool allow-list. Empty requests are omitted and equivalent event/matcher
// pairs are merged deterministically.
func Surface(hooks []adapter.HookSpec) []adapter.HookSpec {
	type grouped struct {
		spec   adapter.HookSpec
		points map[string]bool
		order  int
	}
	groups := make(map[string]*grouped)
	for i, hook := range hooks {
		if hook.Command != "" {
			continue // Native commands never merge with runtime subscriptions.
		}
		event := strings.TrimSpace(hook.NativeEvent)
		if event == "" || len(hook.Points) == 0 {
			continue
		}
		matcher := hook.Matcher
		key := event + "\x00" + matcher
		entry := groups[key]
		if entry == nil {
			entry = &grouped{spec: adapter.HookSpec{NativeEvent: event, Matcher: matcher, Extra: cloneExtra(hook.Extra)}, points: make(map[string]bool), order: i}
			groups[key] = entry
		}
		for _, point := range hook.Points {
			point = strings.TrimSpace(point)
			if point != "" {
				entry.points[point] = true
			}
		}
		if len(entry.spec.Extra) == 0 && len(hook.Extra) > 0 {
			entry.spec.Extra = cloneExtra(hook.Extra)
		}
	}
	out := make([]*grouped, 0, len(groups))
	for _, entry := range groups {
		if len(entry.points) == 0 {
			continue
		}
		for point := range entry.points {
			entry.spec.Points = append(entry.spec.Points, point)
		}
		sort.Slice(entry.spec.Points, func(i, j int) bool {
			return pointOrder(entry.spec.Points[i]) < pointOrder(entry.spec.Points[j]) || (pointOrder(entry.spec.Points[i]) == pointOrder(entry.spec.Points[j]) && entry.spec.Points[i] < entry.spec.Points[j])
		})
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ei, ej := eventOrder(out[i].spec.NativeEvent), eventOrder(out[j].spec.NativeEvent)
		if ei != ej {
			return ei < ej
		}
		if out[i].spec.NativeEvent != out[j].spec.NativeEvent {
			return out[i].spec.NativeEvent < out[j].spec.NativeEvent
		}
		if out[i].spec.Matcher != out[j].spec.Matcher {
			return out[i].spec.Matcher < out[j].spec.Matcher
		}
		return out[i].order < out[j].order
	})
	result := make([]adapter.HookSpec, 0, len(out))
	for _, entry := range out {
		result = append(result, entry.spec)
	}
	return result
}

// DefaultSurface is useful for an installation that has no package-specific
// dynamic requests yet. Runtime filtering still happens after decoding.
func DefaultSurface() []adapter.HookSpec {
	return Surface([]adapter.HookSpec{
		{NativeEvent: "SessionStart", Points: []string{plan.SessionStart}},
		{NativeEvent: "UserPromptSubmit", Points: []string{plan.PromptSubmitted}},
		{NativeEvent: "UserPromptExpansion", Points: []string{plan.SkillExplicit}},
		{NativeEvent: "PreToolUse", Points: []string{plan.ToolPre}},
		{NativeEvent: "PostToolUse", Points: []string{plan.ToolPost}},
		{NativeEvent: "PostToolUseFailure", Points: []string{plan.ToolError}},
		{NativeEvent: "SubagentStart", Points: []string{plan.SubagentStart}},
	})
}

func eventOrder(event string) int {
	for i, name := range []string{"SessionStart", "UserPromptSubmit", "UserPromptExpansion", "PreToolUse", "PostToolUse", "PostToolUseFailure", "SubagentStart", "PostModelSwitch"} {
		if event == name {
			return i
		}
	}
	return 100
}

func pointOrder(point string) int {
	for i, name := range []string{plan.SessionStart, plan.PromptSubmitted, plan.SkillExplicit, plan.ToolPre, plan.ToolPost, plan.ToolError, plan.SubagentStart} {
		if point == name {
			return i
		}
	}
	return 100
}

func cloneExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	copy := make(map[string]any, len(extra))
	for key, value := range extra {
		copy[key] = value
	}
	return copy
}

// Facts implements the provider facts consumed by core/plan.
type Facts struct {
	StaticAppendAvailable bool
	StaticPath            string
}

func DefaultFacts() Facts { return Facts{StaticAppendAvailable: true} }

// NewFacts inspects only the static instruction slot. A malformed existing
// managed region is unavailable so planning can report a safe fallback rather
// than overwriting user bytes.
func NewFacts(installation adapter.Installation) Facts {
	dir := installation.ConfigDir
	if dir == "" {
		dir = defaultClaudeHome()
	}
	path := filepath.Join(dir, "CLAUDE.md")
	data, err := os.ReadFile(path)
	available := true
	if err == nil {
		available = claudeStaticAvailable(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		available = false
	}
	return Facts{StaticAppendAvailable: available, StaticPath: path}
}

func (f Facts) Capability(point string, capability plan.Capability) adapter.CapabilitySupport {
	available := map[string]map[plan.Capability]bool{
		plan.SessionStart:    {plan.AddContext: true, plan.SystemMessage: true},
		plan.PromptSubmitted: {plan.AddContext: true, plan.SystemMessage: true, plan.Deny: true},
		plan.SkillExplicit:   {plan.AddContext: true, plan.SystemMessage: true, plan.Deny: true},
		plan.ToolPre:         {plan.AddContext: true, plan.SystemMessage: true, plan.Deny: true},
		plan.ToolPost:        {plan.AddContext: true, plan.SystemMessage: true},
		plan.ToolError:       {plan.AddContext: true, plan.SystemMessage: true},
	}
	if available[point][capability] {
		return adapter.CapabilitySupport{Available: true, Reason: "Claude Code hook output supports this capability"}
	}
	return adapter.CapabilitySupport{Reason: "Claude Code has no output capability at this evaluation point"}
}

func (f Facts) DataSource(point, field string) adapter.DataAvailability {
	common := map[string]bool{
		"model": true, "thinking_effort": true, "thinking_effort_raw": true,
		"thinking_effort_approx": true, "harness.name": true, "harness.version": true,
		"harness.cwd": true, "harness.permission_mode": true, "harness.session_id": true,
		"harness.turn_id": true, "harness.agent": true, "agent": true,
	}
	if common[field] {
		return adapter.DataAvailability{Available: true, Source: "Claude Code event/session state"}
	}
	switch point {
	case plan.PromptSubmitted:
		if field == "prompt" {
			return adapter.DataAvailability{Available: true, Source: "Claude UserPromptSubmit payload"}
		}
	case plan.SkillExplicit:
		if field == "prompt" || field == "skill.name" {
			return adapter.DataAvailability{Available: true, Source: "Claude UserPromptExpansion payload"}
		}
	case plan.ToolPre:
		if claudeToolFieldAvailable(field, false) {
			return adapter.DataAvailability{Available: true, Source: "Claude PreToolUse payload"}
		}
	case plan.ToolPost:
		if claudeToolFieldAvailable(field, true) {
			return adapter.DataAvailability{Available: true, Source: "Claude PostToolUse payload"}
		}
	case plan.ToolError:
		// Native errors are normalized by the codec, but M1's condition
		// schema exposes tool.output and tool.added only at their specified
		// pre/post points. Do not advertise them for tool.error planning.
		if field != "tool.output" && field != "tool.added" && claudeToolFieldAvailable(field, false) {
			return adapter.DataAvailability{Available: true, Source: "Claude PostToolUseFailure payload"}
		}
	case plan.SubagentStart:
		if field == "agent" || field == "harness.agent" {
			return adapter.DataAvailability{Available: true, Source: "Claude SubagentStart payload"}
		}
	}
	return adapter.DataAvailability{Reason: "Claude Code does not expose this field at the evaluation point"}
}

func claudeToolFieldAvailable(field string, post bool) bool {
	switch field {
	case "tool.name", "tool.canonical", "tool.input", "tool.added", "mcp.server", "mcp.tool", "skill.name", "cwd":
		return true
	case "tool.output":
		return post
	}
	return strings.HasPrefix(field, "tool.input.")
}

// Claude can subscribe to native tool names dynamically; no canonical tool
// selector is inherently unobservable at this layer.
func (f Facts) BlindSpot(_ spec.Rule) (string, bool) { return "", false }

func (f Facts) StaticLocation(mode string) (adapter.StaticLocation, bool) {
	if mode != "append" || !f.StaticAppendAvailable {
		return adapter.StaticLocation{Mode: mode, Name: StaticSlotAppend, Reason: "CLAUDE.md is unavailable or contains malformed managed markers"}, false
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

// Detector discovers Claude Code without writing configuration.
type Detector struct {
	Home     string
	LookPath func(string) (string, error)
	Run      func(context.Context, string, ...string) ([]byte, error)
}

func (d Detector) Detect(ctx context.Context) (adapter.Installation, bool, error) {
	home := d.Home
	if home == "" {
		home = os.Getenv("CLAUDE_CONFIG_DIR")
	}
	if home == "" {
		home = defaultClaudeHome()
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
	path, pathErr := lookPath("claude")
	_, settingsErr := os.Stat(filepath.Join(home, "settings.json"))
	_, staticErr := os.Stat(filepath.Join(home, "CLAUDE.md"))
	if pathErr != nil && settingsErr != nil && staticErr != nil {
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

// Definition returns the offline Claude adapter definition.
func Definition() adapter.Definition {
	return DefinitionForInstallation(adapter.Installation{Provider: ProviderName, ConfigDir: defaultClaudeHome()})
}

func DefinitionForInstallation(installation adapter.Installation) adapter.Definition {
	if installation.ConfigDir == "" {
		installation.ConfigDir = defaultClaudeHome()
	}
	installation.Provider = ProviderName
	facts := NewFacts(installation)
	return adapter.Definition{
		ID:              ProviderName,
		Detector:        Detector{},
		Facts:           facts,
		StaticPositions: facts,
		Routes:          []adapter.Route{NewUserRouteForInstallation(installation.ConfigDir, installation.Version, "", &facts)},
		Limits:          adapter.Limits{Input: tokens.InputLimit{Value: DefaultInputChars, Unit: tokens.CharsUnit}, Approximate: false, Source: "Claude Code hook input limit", Overflow: adapter.OverflowFilePreview},
		Compact:         adapter.CompactSignal{Event: agenthooks.KindSessionStart, SessionSource: "compact", Deferred: true},
		Native:          NativeSurface{},
	}
}

func NewDefinition() adapter.Definition { return Definition() }

// HookManifest translates normalized provider-neutral subscriptions into the
// shared installation renderer. Native events and matchers are retained so
// Claude can subscribe to PostModelSwitch and tool names directly.
func HookManifest(command []string, requests ...[]adapter.HookSpec) install.Manifest {
	hooks := DefaultSurface()
	if len(requests) > 0 {
		hooks = requests[0]
	}
	normalized := Surface(hooks)
	manifestHooks := make([]install.HookSpec, 0, len(normalized))
	for _, surface := range normalized {
		blocking := surface.NativeEvent != "PostModelSwitch" && surface.NativeEvent != "SubagentStart"
		spec := install.HookSpec{Kind: nativeKind(surface.NativeEvent), NativeEvent: surface.NativeEvent, NativeMatcher: surface.Matcher, Blocking: blocking}
		if value, ok := surface.Extra["blocking"].(bool); ok {
			spec.Blocking = value
		}
		if value, ok := surface.Extra["timeout"].(time.Duration); ok && value > 0 {
			spec.Timeout = value
		}
		if value, ok := surface.Extra["timeout_seconds"].(int); ok && value > 0 {
			spec.Timeout = time.Duration(value) * time.Second
		}
		manifestHooks = append(manifestHooks, spec)
	}
	for _, hook := range hooks {
		if hook.Command == "" {
			continue
		}
		seconds, _ := hook.Extra["timeout_seconds"].(int)
		manifestHooks = append(manifestHooks, install.HookSpec{NativeEvent: hook.NativeEvent, NativeMatcher: hook.Matcher,
			Blocking: true, Timeout: time.Duration(seconds) * time.Second, RawCommand: hook.Command, RawTag: hook.NativeTag})
	}
	manifest := install.Manifest{Command: append([]string(nil), command...), Hooks: manifestHooks, Identity: install.Identity{Name: "style-runtime", Version: "0.1.0", Description: "STYLE runtime hooks"}, Owner: Owner}
	if value, ok := firstExtra(normalized, "fail").(agenthooks.FailMode); ok && value == agenthooks.FailClosed {
		manifest.Fail = agenthooks.FailClosed
	}
	if value, ok := firstExtra(normalized, "fail_closed").(bool); ok && value {
		manifest.Fail = agenthooks.FailClosed
	}
	return manifest
}

func firstExtra(hooks []adapter.HookSpec, key string) any {
	for _, hook := range hooks {
		if value, ok := hook.Extra[key]; ok {
			return value
		}
	}
	return nil
}

func nativeKind(native string) agenthooks.EventKind {
	for name, kind := range map[string]agenthooks.EventKind{
		"SessionStart": agenthooks.KindSessionStart, "UserPromptSubmit": agenthooks.KindPromptSubmitted,
		"UserPromptExpansion": agenthooks.KindPromptExpansion, "PreToolUse": agenthooks.KindToolPre,
		"PostToolUse": agenthooks.KindToolPost, "PostToolUseFailure": agenthooks.KindToolError,
		"PermissionRequest": agenthooks.KindPermission, "SubagentStart": agenthooks.KindSubagentStart,
	} {
		if native == name {
			return kind
		}
	}
	return agenthooks.KindOther
}

func defaultClaudeHome() string {
	if home := os.Getenv("CLAUDE_CONFIG_DIR"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

var _ adapter.PlanningFacts = Facts{}
var _ adapter.StaticPositionProvider = Facts{}
var _ adapter.Detector = Detector{}
