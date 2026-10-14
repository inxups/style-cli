package install

import (
	"errors"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	"github.com/speakeasy-api/agenthooks"
)

// kindToClaude maps unified kinds to Claude Code hook event names.
// model.request has no mapping; model.response ≈ MessageDisplay is excluded
// from generation by default (§12.3).
var kindToClaude = map[agenthooks.EventKind]string{
	agenthooks.KindSessionStart:    "SessionStart",
	agenthooks.KindSessionEnd:      "SessionEnd",
	agenthooks.KindPromptSubmitted: "UserPromptSubmit",
	agenthooks.KindPromptExpansion: "UserPromptExpansion",
	agenthooks.KindToolPre:         "PreToolUse",
	agenthooks.KindToolPost:        "PostToolUse",
	agenthooks.KindToolError:       "PostToolUseFailure",
	agenthooks.KindPermission:      "PermissionRequest",
	agenthooks.KindStop:            "Stop",
	agenthooks.KindSubagentStart:   "SubagentStart",
	agenthooks.KindSubagentStop:    "SubagentStop",
	agenthooks.KindCompactPre:      "PreCompact",
	agenthooks.KindCompactPost:     "PostCompact",
	agenthooks.KindNotification:    "Notification",
	agenthooks.KindFileEdited:      "FileChanged",
}

type claudeHookCmd struct {
	Type      string   `json:"type"`
	Command   string   `json:"command"`
	Args      []string `json:"args,omitempty"`
	Timeout   int      `json:"timeout,omitempty"`
	Async     bool     `json:"async,omitempty"`
	OnFailure string   `json:"onFailure,omitempty"`
}

type claudeMatcherEntry struct {
	Matcher string          `json:"matcher,omitempty"`
	Hooks   []claudeHookCmd `json:"hooks"`
}

func renderClaude(m Manifest, t Target) (fs.FS, error) {
	if len(m.Command) == 0 || strings.TrimSpace(m.Command[0]) == "" {
		return nil, errors.New("install: Claude Code command executable is required")
	}
	if needsClaudeFailClosed(m) && !claudeSupportsOnFailure(t.HarnessVersion) {
		return nil, claudeVersionError(t.HarnessVersion)
	}
	return renderClaudeConfig(m, t)
}

// renderClaudeForUninstall skips new-install compatibility checks because
// removal only needs the provider paths and owner marker. In particular, a
// Claude downgrade must not strand a previously installed fail-closed hook.
func renderClaudeForUninstall(m Manifest, t Target) (fs.FS, error) {
	return renderClaudeConfig(m, t)
}

func renderClaudeConfig(m Manifest, t Target) (fs.FS, error) {
	hooks := map[string][]claudeMatcherEntry{}
	for _, spec := range m.Hooks {
		event, ok := spec.NativeEvent, spec.NativeEvent != ""
		if !ok {
			event, ok = kindToClaude[spec.Kind]
		}
		if !ok {
			continue
		}
		matcher, err := claudeMatcher(spec)
		if err != nil {
			return nil, err
		}
		async := !spec.Blocking
		// cowork drops async Stop hooks (quirk #1): Stop is forced sync.
		if event == "Stop" {
			async = false
		}
		command := ""
		var args []string
		if len(m.Command) > 0 {
			command = m.Command[0]
			args = append(args, m.Command[1:]...)
		}
		args = append(args, hookArgs(m, agenthooks.ProviderClaudeCode, spec)...)
		onFailure := ""
		if isClaudeFailClosedPreTool(m, spec) {
			onFailure = "block"
			args = append(args, "--fail-closed")
		}
		hooks[event] = append(hooks[event], claudeMatcherEntry{
			Matcher: matcher,
			Hooks: []claudeHookCmd{{
				Type:      "command",
				Command:   command,
				Args:      args,
				Timeout:   timeoutSeconds(spec),
				Async:     async,
				OnFailure: onFailure,
			}},
		})
	}

	files := map[string][]byte{}
	hooksJSON, err := jsonFile(map[string]any{"hooks": hooks})
	if err != nil {
		return nil, err
	}

	switch t.Scope {
	case ScopePlugin:
		if m.Identity.Name == "" {
			return nil, errors.New("install: plugin scope requires Identity.Name")
		}
		pluginJSON, err := jsonFile(map[string]string{
			"name":        m.Identity.Name,
			"version":     m.Identity.Version,
			"description": m.Identity.Description,
		})
		if err != nil {
			return nil, err
		}
		files[".claude-plugin/plugin.json"] = pluginJSON
		// Must be at hooks/hooks.json, not the plugin root (§7).
		files["hooks/hooks.json"] = hooksJSON
	case ScopeProject:
		files[".claude/settings.json"] = hooksJSON
	default: // ScopeUser: Target.Dir is ~/.claude
		files["settings.json"] = hooksJSON
	}
	return memFS(files), nil
}

func claudeMatcher(spec HookSpec) (string, error) {
	if spec.NativeMatcher != "" {
		if _, err := regexp.Compile(spec.NativeMatcher); err != nil {
			return "", errors.New("install: invalid native Claude matcher " + strconv.Quote(spec.NativeMatcher))
		}
		return spec.NativeMatcher, nil
	}
	matcher, _ := agenthooks.CompileMatcher(agenthooks.ProviderClaudeCode, spec.Tools)
	return matcher, nil
}

func needsClaudeFailClosed(m Manifest) bool {
	for _, spec := range m.Hooks {
		if isClaudeFailClosedPreTool(m, spec) {
			return true
		}
	}
	return false
}

func isClaudeFailClosedPreTool(m Manifest, spec HookSpec) bool {
	return m.Fail == agenthooks.FailClosed && spec.Blocking && spec.Kind == agenthooks.KindToolPre
}

func claudeVersionError(version string) error {
	if strings.TrimSpace(version) == "" {
		return errors.New("install: Claude Code version is required for fail-closed PreToolUse hooks (requires >= 2.1.295)")
	}
	return errors.New("install: Claude Code " + version + " does not support fail-closed PreToolUse hooks (requires >= 2.1.295)")
}

func claudeSupportsOnFailure(version string) bool {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" {
		return false
	}
	// `claude --version` may append a human-readable product label.
	version = strings.Fields(version)[0]
	// Ignore build metadata. A prerelease of the minimum version is still
	// below the stable onFailure support floor.
	version = strings.SplitN(version, "+", 2)[0]
	prerelease := strings.Contains(version, "-")
	version = strings.SplitN(version, "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	values := [3]int{}
	for i, part := range parts {
		if part == "" {
			return false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return false
		}
		values[i] = value
	}
	minimum := [3]int{2, 1, 295}
	for i := range values {
		if values[i] != minimum[i] {
			return values[i] > minimum[i]
		}
	}
	return !prerelease
}
