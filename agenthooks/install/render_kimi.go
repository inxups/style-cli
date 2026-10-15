package install

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/speakeasy-api/agenthooks"
)

var kindToKimi = map[agenthooks.EventKind]string{
	agenthooks.KindSessionStart:    "SessionStart",
	agenthooks.KindSessionEnd:      "SessionEnd",
	agenthooks.KindPromptSubmitted: "UserPromptSubmit",
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
}

// TOML managed-region markers, shared by the Kimi and Codex renderers: TOML
// configs can't use the JSON settings-merge, so installs own the region
// between these markers and leave everything outside it untouched.
const (
	tomlBeginMarker    = "# BEGIN agenthooks managed hooks (generated; do not edit inside markers)"
	tomlEndMarker      = "# END agenthooks managed hooks"
	tomlBoundaryMarker = "# agenthooks managed hooks separator (generated)"
)

func tomlMarkers(owner string) (begin, end string) {
	if owner == "" || owner == "agenthooks" {
		return tomlBeginMarker, tomlEndMarker
	}
	return "# BEGIN agenthooks managed hooks (owner=" + owner + "; generated; do not edit inside markers)",
		"# END agenthooks managed hooks (owner=" + owner + ")"
}

// renderKimi emits [[hooks]] entries for $KIMI_CODE_HOME/config.toml
// (default ~/.kimi-code/config.toml). Kimi reads hooks from the single
// user-level config only — .kimi-code/local.toml holds [workspace] settings,
// not hooks (verified against kimi-code 0.22.2) — so project scope is
// rejected; per-project isolation goes through KIMI_CODE_HOME instead.
// Timeouts are seconds, clamped to Kimi's documented 1–600 range. There is
// no async or failClosed switch to bake in: the runtime is hard fail-open
// (quirk #21).
func renderKimi(m Manifest, t Target) (fs.FS, error) {
	if t.Scope == ScopeProject {
		return nil, errors.New("install: kimi has no project-level hooks config; use ScopeUser with Dir pointed at $KIMI_CODE_HOME")
	}
	var b strings.Builder
	begin, end := tomlMarkers(manifestOwner(m))
	b.WriteString(begin + "\n")
	for _, spec := range m.Hooks {
		event, ok := kindToKimi[spec.Kind]
		if !ok {
			continue
		}
		matcher, _ := agenthooks.CompileMatcher(agenthooks.ProviderKimi, spec.Tools)
		secs := timeoutSeconds(spec)
		if secs > 600 {
			secs = 600
		}
		b.WriteString("\n[[hooks]]\n")
		fmt.Fprintf(&b, "event = %s\n", tomlString(event))
		if matcher != "" {
			fmt.Fprintf(&b, "matcher = %s\n", tomlString(matcher))
		}
		fmt.Fprintf(&b, "command = %s\n", tomlString(hookCommand(m, agenthooks.ProviderKimi, spec)))
		fmt.Fprintf(&b, "timeout = %d\n", secs)
	}
	b.WriteString("\n" + end + "\n")

	return memFS(map[string][]byte{
		"config.toml": []byte(b.String()), // Target.Dir is ~/.kimi-code (or $KIMI_CODE_HOME)
	}), nil
}

// tomlString renders a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// mergeManagedTOML overlays the rendered managed region onto an existing TOML
// file: the region between the agenthooks markers is replaced; everything
// else survives byte-for-byte. Absent markers, the region is appended.
func mergeManagedTOML(existing, rendered []byte) []byte {
	ex := string(existing)
	beginMarker, endMarker := markersInTOML(rendered)
	region := strings.TrimSuffix(string(rendered), "\n")
	begin := strings.Index(ex, beginMarker)
	if begin < 0 {
		if len(ex) > 0 && !strings.HasSuffix(ex, "\n") {
			ex += "\n" + tomlBoundaryMarker + "\n"
		}
		return []byte(ex + region)
	}
	end := strings.Index(ex[begin:], endMarker)
	if end < 0 {
		// Broken region (begin without end): replace from begin to EOF.
		return []byte(ex[:begin] + region)
	}
	return []byte(ex[:begin] + region + ex[begin+end+len(endMarker):])
}

func removeManagedTOML(existing []byte, owner string) ([]byte, bool) {
	beginMarker, endMarker := tomlMarkers(owner)
	ex := string(existing)
	begin := strings.Index(ex, beginMarker)
	if begin < 0 {
		return existing, false
	}
	end := strings.Index(ex[begin+len(beginMarker):], endMarker)
	if end < 0 {
		return []byte(ex[:begin]), true
	}
	end += begin + len(beginMarker)
	prefix := ex[:begin]
	boundary := "\n" + tomlBoundaryMarker + "\n"
	if strings.HasSuffix(prefix, boundary) {
		prefix = strings.TrimSuffix(prefix, boundary)
	}
	return []byte(prefix + ex[end+len(endMarker):]), true
}

func markersInTOML(rendered []byte) (begin, end string) {
	for _, line := range strings.Split(string(rendered), "\n") {
		switch {
		case strings.HasPrefix(line, "# BEGIN agenthooks managed hooks"):
			begin = line
		case strings.HasPrefix(line, "# END agenthooks managed hooks"):
			end = line
		}
	}
	if begin == "" || end == "" {
		return tomlBeginMarker, tomlEndMarker
	}
	return begin, end
}
