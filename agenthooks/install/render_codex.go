package install

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/speakeasy-api/agenthooks"
)

var kindToCodex = map[agenthooks.EventKind]string{
	agenthooks.KindSessionStart:    "SessionStart",
	agenthooks.KindSessionEnd:      "SessionEnd",
	agenthooks.KindPromptSubmitted: "UserPromptSubmit",
	agenthooks.KindToolPre:         "PreToolUse",
	agenthooks.KindToolPost:        "PostToolUse",
	agenthooks.KindPermission:      "PermissionRequest",
	agenthooks.KindStop:            "Stop",
	agenthooks.KindSubagentStart:   "SubagentStart",
	agenthooks.KindSubagentStop:    "SubagentStop",
	agenthooks.KindCompactPre:      "PreCompact",
	agenthooks.KindCompactPost:     "PostCompact",
}

func renderCodex(m Manifest, t Target) (fs.FS, error) {
	plugin := t.Scope == ScopePlugin
	if plugin && (m.Identity.Name == "" || m.Identity.PluginID == "") {
		return nil, fmt.Errorf("install: Codex plugin scope requires Identity.Name and Identity.PluginID")
	}
	if plugin && strings.ContainsAny(m.Identity.PluginID, ":/\\\r\n\t ") {
		return nil, fmt.Errorf("install: invalid Codex Identity.PluginID %q", m.Identity.PluginID)
	}
	if plugin && runtime.GOOS == "windows" {
		return nil, fmt.Errorf("install: Codex plugin launcher requires a POSIX host")
	}
	hooks := map[string][]claudeMatcherEntry{} // Codex uses the Claude hooks.json shape
	type trustEntry struct {
		key  string
		hash string
	}
	var trust []trustEntry
	// Codex keys hook state by "<source>:<event_label>:<group>:<handler>"
	// where source is the absolute path of the hooks.json it discovered, so
	// Target.Dir must be the CODEX_HOME the config is installed into. Codex
	// canonicalizes that path, so symlinks (e.g. /var -> /private/var on
	// macOS) must be resolved or the state keys never match.
	dir := t.Dir
	if plugin {
		dir = t.ConfigDir
	}
	if dir == "" {
		return nil, fmt.Errorf("install: Codex plugin scope requires Target.ConfigDir")
	}
	dir = canonicalCodexDir(dir)
	source := filepath.Join(dir, "hooks.json")
	if plugin {
		source = filepath.Join("hooks", "hooks.json")
	}
	for _, spec := range m.Hooks {
		event, ok := kindToCodex[spec.Kind]
		if !ok {
			continue
		}
		matcher, err := codexMatcher(spec)
		if err != nil {
			return nil, fmt.Errorf("install: Codex %s matcher: %w", event, err)
		}
		command := hookCommand(m, agenthooks.ProviderCodex, spec)
		if plugin {
			command = codexPluginHookCommand(m, spec)
		}
		// Codex parses-but-skips async:true (quirk #10): telemetry hooks get
		// --async, which makes the runner re-exec itself as a detached worker
		// and return immediately — no shell involved.
		if !spec.Blocking {
			command += " --async"
		}
		secs := timeoutSeconds(spec)
		if event == "SessionEnd" && secs > 3 {
			secs = 3
		}
		key := fmt.Sprintf("%s:%s:%d:0", source, codexEventLabel[event], len(hooks[event]))
		if plugin {
			key = fmt.Sprintf("%s:%s:%d:0", m.Identity.PluginID, filepath.ToSlash(source)+":"+codexEventLabel[event], len(hooks[event]))
		}
		trust = append(trust, trustEntry{
			key:  key,
			hash: DefinitionHash(event, matcher, command, secs),
		})
		hooks[event] = append(hooks[event], claudeMatcherEntry{
			Matcher: matcher,
			Hooks: []claudeHookCmd{{
				Type:    "command",
				Command: command,
				Timeout: secs,
			}},
		})
	}
	hooksJSON, err := jsonFile(map[string]any{"hooks": hooks})
	if err != nil {
		return nil, err
	}

	// Codex hooks require user trust of the definition hash (quirk #9):
	// installs pre-seed [hooks.state] tables so generated hooks run without
	// an interactive trust prompt. Rendered into config.toml inside the
	// managed marker region (merged like Kimi's TOML configs).
	sort.Slice(trust, func(i, j int) bool { return trust[i].key < trust[j].key })
	var toml strings.Builder
	begin, end := tomlMarkers(manifestOwner(m))
	toml.WriteString(begin + "\n")
	for _, e := range trust {
		fmt.Fprintf(&toml, "\n[hooks.state.%s]\n", tomlString(e.key))
		fmt.Fprintf(&toml, "trusted_hash = %s\n", tomlString(e.hash))
	}
	// Keep the generated region self-contained, just like mergeManagedTOML.
	// A newline after the end marker would survive owner removal and leave a
	// formerly absent config.toml behind after STYLE removes its static block.
	toml.WriteString("\n" + end)

	files := map[string][]byte{"config.toml": []byte(toml.String())}
	if plugin {
		pluginJSON, err := jsonFile(map[string]string{
			"name": m.Identity.Name, "version": m.Identity.Version, "description": m.Identity.Description,
		})
		if err != nil {
			return nil, err
		}
		files[filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json"))] = pluginJSON
		files[filepath.ToSlash(filepath.Join("hooks", "hooks.json"))] = hooksJSON
		files[filepath.ToSlash(filepath.Join("bin", "style"))] = []byte(codexPluginLauncher(m.Command))
	} else {
		files["hooks.json"] = hooksJSON
	}
	if plugin {
		return memFSWithModes(files, map[string]fs.FileMode{
			filepath.ToSlash(filepath.Join("bin", "style")): 0o755,
		}), nil
	}
	return memFS(files), nil
}

func codexMatcher(spec HookSpec) (string, error) {
	if spec.NativeMatcher == "" {
		matcher, _ := agenthooks.CompileMatcher(agenthooks.ProviderCodex, spec.Tools)
		return matcher, nil
	}
	if _, err := regexp.Compile(spec.NativeMatcher); err != nil {
		return "", fmt.Errorf("invalid native matcher %q: %w", spec.NativeMatcher, err)
	}
	return spec.NativeMatcher, nil
}

func codexPluginLauncher(command []string) string {
	parts := make([]string, len(command))
	for i, arg := range command {
		parts[i] = shellQuote(arg)
	}
	return "#!/bin/sh\nexec " + strings.Join(parts, " ") + " \"$@\"\n"
}

func codexPluginHookCommand(m Manifest, spec HookSpec) string {
	args := hookArgs(m, agenthooks.ProviderCodex, spec)
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = shellQuote(arg)
	}
	command := `exec "$PLUGIN_ROOT/bin/style" ` + strings.Join(parts, " ")
	return command
}
