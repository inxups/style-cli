// Package install renders and installs provider hook configurations from a
// single Go Manifest: correct hooks.json / settings.json / config.toml /
// plugin scaffolding per provider, with the per-provider timing/async/
// fail-mode workarounds baked in (DESIGN.md §7).
package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/speakeasy-api/agenthooks"
)

// Scope selects where the configuration lands.
type Scope string

const (
	ScopeUser    Scope = "user"    // provider's user-level config dir
	ScopeProject Scope = "project" // repo-relative config
	ScopePlugin  Scope = "plugin"  // plugin-based install (Claude Code)
)

// Target names a provider plus scope; Dir is the filesystem root Install
// writes under (ignored by Render, which returns a relative fs.FS).
type Target struct {
	Provider agenthooks.Provider
	Scope    Scope
	Dir      string
	// HarnessVersion is the detected provider version. Claude Code requires
	// this to be known and at least 2.1.295 before rendering fail-closed
	// PreToolUse hooks, which rely on its onFailure setting.
	HarnessVersion string
	// ConfigDir is the provider's user config root when the rendered files
	// also include a separately installed plugin bundle. Codex ScopePlugin
	// writes its plugin under Dir and trust state under ConfigDir.
	ConfigDir string
}

// Identity is the plugin name/version/description for plugin-based installs
// and for Gemini's /hooks enable|disable UX.
type Identity struct {
	Name        string
	Version     string
	Description string
	// PluginID is Codex's namespaced plugin identifier, such as
	// "style-runtime@style". It is used in Codex's long-form hook trust keys.
	PluginID string
}

// ToolMatcher aliases the unified matcher (compiled to provider dialects
// where expressible, enforced in-process via --filter where not).
type ToolMatcher = agenthooks.ToolMatcher

// HookSpec subscribes one unified event kind.
type HookSpec struct {
	Kind agenthooks.EventKind
	// NativeEvent optionally names a provider event directly. This is useful
	// for provider events that have no unified EventKind (for example Claude
	// Code's PostModelSwitch observer). When set, the provider renderer uses it
	// instead of deriving an event name from Kind.
	NativeEvent string
	// Tools is the provider-neutral matcher. NativeMatcher optionally pins a
	// provider-native matcher for renderers that support raw patterns (Codex
	// and Claude); when set, that renderer uses it verbatim instead of
	// compiling Tools.
	Tools         ToolMatcher
	NativeMatcher string
	Blocking      bool // decision path vs telemetry path
	Timeout       time.Duration
}

// Manifest is the single source from which every provider config is
// generated.
type Manifest struct {
	Command  []string // how to invoke the consumer binary (abs path or PATH lookup)
	Hooks    []HookSpec
	Identity Identity
	// Owner distinguishes this consumer's hook entries from other programs
	// using agenthooks. An empty owner defaults to "agenthooks" for existing
	// consumers; STYLE should set it to "style".
	Owner string
	// Fail drives fail-mode workarounds in generated config (e.g. Cursor's
	// failClosed flag, quirk #7). Match the Policy your Runner uses.
	Fail agenthooks.FailMode
}

// Render produces the provider configuration as an in-memory filesystem with
// target-relative paths. Kinds a provider cannot subscribe to (see the
// mapping table, §3.4) are skipped; model.request/model.response are excluded
// from generation by design (§12.3).
func Render(m Manifest, t Target) (fs.FS, error) {
	if len(m.Command) == 0 {
		return nil, errors.New("install: Manifest.Command is required")
	}
	if _, err := ownerName(m); err != nil {
		return nil, err
	}
	switch t.Provider {
	case agenthooks.ProviderClaudeCode:
		return renderClaude(m, t)
	case agenthooks.ProviderCursor:
		return renderCursor(m, t)
	case agenthooks.ProviderCodex:
		return renderCodex(m, t)
	case agenthooks.ProviderGemini:
		return renderGemini(m, t)
	case agenthooks.ProviderOpenCode:
		return renderOpenCode(m, t)
	case agenthooks.ProviderOpenClaw:
		return renderOpenClaw(m, t)
	case agenthooks.ProviderKimi:
		return renderKimi(m, t)
	case agenthooks.ProviderCopilotCLI:
		return renderCopilot(m, t)
	case agenthooks.ProviderVSCodeCopilot:
		return renderVSCode(m, t)
	}
	return nil, fmt.Errorf("install: unknown provider %q", t.Provider)
}

// ChangeState classifies one file in a Diff.
type ChangeState string

const (
	StateCreate    ChangeState = "create"
	StateUpdate    ChangeState = "update"
	StateUnchanged ChangeState = "unchanged"
)

// Change is one file-level difference between the rendered config and disk.
type Change struct {
	Path  string
	State ChangeState
}

// InstallOption configures Install.
type InstallOption func(*installCfg)

type installCfg struct {
	dryRun bool
}

// WithDryRun plans without writing.
func WithDryRun() InstallOption {
	return func(c *installCfg) { c.dryRun = true }
}

// Install renders the manifest and writes it under t.Dir, idempotently:
// unchanged files are skipped, and JSON settings files are merged so
// agenthooks-managed entries are replaced while foreign entries survive.
func Install(ctx context.Context, m Manifest, t Target, opts ...InstallOption) error {
	var cfg installCfg
	for _, o := range opts {
		o(&cfg)
	}
	planned, err := PlanFiles(m, t)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg.dryRun {
		return nil
	}
	return ApplyFiles(ctx, planned)
}

// Diff reports what Install would do, without writing.
func Diff(m Manifest, t Target) ([]Change, error) {
	planned, err := plan(m, t)
	if err != nil {
		return nil, err
	}
	out := make([]Change, len(planned))
	for i, p := range planned {
		out[i] = Change{Path: p.path, State: p.state}
	}
	return out, nil
}

// Fingerprint hashes the rendered configuration — the idempotence key for
// "re-install only when the manifest changed" flows.
func Fingerprint(m Manifest, t Target) (string, error) {
	rendered, err := Render(m, t)
	if err != nil {
		return "", err
	}
	paths, contents, err := flatten(rendered)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for i, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(contents[i])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type plannedFile struct {
	path   string
	root   string
	before []byte
	final  []byte
	mode   fs.FileMode
	state  ChangeState
}

func plan(m Manifest, t Target) ([]plannedFile, error) {
	return planWithReader(m, t, os.ReadFile)
}

func planWithReader(m Manifest, t Target, readFile func(string) ([]byte, error)) ([]plannedFile, error) {
	rendered, err := Render(m, t)
	if err != nil {
		return nil, err
	}
	paths, contents, err := flatten(rendered)
	if err != nil {
		return nil, err
	}
	var out []plannedFile
	for i, p := range paths {
		content := contents[i]
		info, err := fs.Stat(rendered, p)
		if err != nil {
			return nil, err
		}
		mode := info.Mode().Perm()
		root := t.Dir
		if t.Provider == agenthooks.ProviderCodex && t.Scope == ScopePlugin && p == "config.toml" {
			root = t.ConfigDir
		}
		dest := filepath.Join(root, filepath.FromSlash(p))
		existing, readErr := readFile(dest)
		switch {
		case errors.Is(readErr, os.ErrNotExist):
			out = append(out, plannedFile{path: p, root: root, final: content, mode: mode, state: StateCreate})
			if isMergeableJSON(p) {
				restore, err := planJSONRestore(root, p, manifestOwner(m), nil, content, readFile)
				if err != nil {
					return nil, err
				}
				if restore != nil {
					out = append(out, *restore)
				}
			}
		case readErr != nil:
			return nil, fmt.Errorf("install: read %s: %w", p, readErr)
		default:
			final := content
			if isMergeableJSON(p) {
				merged, mergeErr := mergeManagedJSON(existing, content, manifestOwner(m))
				if mergeErr != nil {
					return nil, fmt.Errorf("install: merge %s: %w", p, mergeErr)
				}
				final = merged
			} else if isMergeableTOML(p) {
				if t.Provider == agenthooks.ProviderCodex {
					final, err = mergeCodexManagedTOML(existing, content, manifestOwner(m))
					if err != nil {
						return nil, fmt.Errorf("install: merge %s: %w", p, err)
					}
				} else {
					final = mergeManagedTOML(existing, content)
				}
			}
			state := StateUpdate
			fileInfo, statErr := os.Stat(dest)
			modeMatches := mode&0o111 == 0 || statErr == nil && fileInfo.Mode().Perm() == mode
			if bytes.Equal(existing, final) && modeMatches {
				state = StateUnchanged
			}
			out = append(out, plannedFile{path: p, root: root, before: cloneBytes(existing), final: final, mode: mode, state: state})
			if isMergeableJSON(p) {
				restore, err := planJSONRestore(root, p, manifestOwner(m), existing, final, readFile)
				if err != nil {
					return nil, err
				}
				if restore != nil {
					out = append(out, *restore)
				}
			}
		}
	}
	return out, nil
}

func removeOwnedJSON(existing []byte, owner string) (updated []byte, removeFile, changed bool, err error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, false, false, err
	}
	rawHooks, ok := root["hooks"]
	if !ok {
		return existing, false, false, nil
	}
	var hooks map[string][]json.RawMessage
	if err := json.Unmarshal(rawHooks, &hooks); err != nil {
		return nil, false, false, err
	}
	changed = false
	for event, groups := range hooks {
		kept, err := removeOwnedHookGroups(groups, owner)
		if err != nil {
			return nil, false, false, err
		}
		if !rawGroupsEqual(kept, groups) {
			changed = true
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !changed {
		return existing, false, false, nil
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		encoded, err := json.Marshal(hooks)
		if err != nil {
			return nil, false, false, err
		}
		root["hooks"] = encoded
	}
	if len(root) == 0 {
		return nil, true, true, nil
	}
	updated, err = json.MarshalIndent(root, "", "  ")
	return updated, false, true, err
}

func rawGroupsEqual(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func pruneEmptyDirs(root, dir string) {
	root, dir = filepath.Clean(root), filepath.Clean(dir)
	for dir != root && dir != "." {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func flatten(fsys fs.FS) (paths []string, contents [][]byte, err error) {
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		b, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return readErr
		}
		paths = append(paths, path)
		contents = append(contents, b)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Sort(&pairSort{paths, contents})
	return paths, contents, nil
}

type pairSort struct {
	paths    []string
	contents [][]byte
}

func (s *pairSort) Len() int           { return len(s.paths) }
func (s *pairSort) Less(i, j int) bool { return s.paths[i] < s.paths[j] }
func (s *pairSort) Swap(i, j int) {
	s.paths[i], s.paths[j] = s.paths[j], s.paths[i]
	s.contents[i], s.contents[j] = s.contents[j], s.contents[i]
}

func renderedRoot(t Target, path string) string {
	if t.Provider == agenthooks.ProviderCodex && t.Scope == ScopePlugin && path == "config.toml" {
		return t.ConfigDir
	}
	return t.Dir
}

func isMergeableJSON(path string) bool {
	base := filepath.Base(path)
	return base == "settings.json" || base == "hooks.json"
}

// isMergeableTOML matches Kimi's shared config files, merged via the managed
// marker region (render_kimi.go). Codex's agenthooks-trust.toml is fully
// owned by agenthooks and deliberately excluded.
func isMergeableTOML(path string) bool {
	base := filepath.Base(path)
	return base == "config.toml" || base == "local.toml"
}

// mergeManagedJSON overlays rendered config onto an existing settings file.
// Only command handlers carrying the same --owner value are replaced; foreign
// handlers, including other agenthooks consumers, survive.
func mergeManagedJSON(existing, rendered []byte, owner string) ([]byte, error) {
	if bytes.Equal(existing, rendered) {
		return existing, nil
	}
	var ex, re map[string]json.RawMessage
	if err := json.Unmarshal(existing, &ex); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rendered, &re); err != nil {
		return nil, err
	}
	for k, v := range re {
		if k != "hooks" {
			ex[k] = v
			continue
		}
		var exHooks, reHooks map[string][]json.RawMessage
		if err := json.Unmarshal(v, &reHooks); err != nil {
			ex[k] = v
			continue
		}
		if raw, ok := ex[k]; ok {
			if err := json.Unmarshal(raw, &exHooks); err != nil {
				exHooks = nil
			}
		}
		if exHooks == nil {
			exHooks = map[string][]json.RawMessage{}
		}
		for event, entries := range exHooks {
			kept, err := removeOwnedHookGroups(entries, owner)
			if err != nil {
				return nil, err
			}
			if len(kept) == 0 {
				delete(exHooks, event)
			} else {
				exHooks[event] = kept
			}
		}
		for event, entries := range reHooks {
			exHooks[event] = append(exHooks[event], entries...)
		}
		if len(exHooks) == 0 {
			delete(ex, k)
		} else {
			merged, err := json.Marshal(exHooks)
			if err != nil {
				return nil, err
			}
			ex[k] = merged
		}
	}
	return json.MarshalIndent(ex, "", "  ")
}

func removeOwnedHookGroups(groups []json.RawMessage, owner string) ([]json.RawMessage, error) {
	kept := make([]json.RawMessage, 0, len(groups))
	for _, rawGroup := range groups {
		var group map[string]json.RawMessage
		if err := json.Unmarshal(rawGroup, &group); err != nil {
			kept = append(kept, rawGroup)
			continue
		}
		var handlers []json.RawMessage
		if rawHandlers := group["hooks"]; len(rawHandlers) != 0 {
			if err := json.Unmarshal(rawHandlers, &handlers); err != nil {
				kept = append(kept, rawGroup)
				continue
			}
		}
		remaining := make([]json.RawMessage, 0, len(handlers))
		for _, rawHandler := range handlers {
			var handler struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			}
			if err := json.Unmarshal(rawHandler, &handler); err != nil || !handlerHasOwner(handler.Command, handler.Args, owner) {
				remaining = append(remaining, rawHandler)
			}
		}
		if len(remaining) == len(handlers) {
			kept = append(kept, rawGroup)
			continue
		}
		if len(remaining) == 0 {
			continue
		}
		updated, err := json.Marshal(remaining)
		if err != nil {
			return nil, err
		}
		group["hooks"] = updated
		updatedGroup, err := json.Marshal(group)
		if err != nil {
			return nil, err
		}
		kept = append(kept, updatedGroup)
	}
	return kept, nil
}

func commandHasOwner(command, owner string) bool {
	if command == "" || owner == "" {
		return false
	}
	want := "--owner=" + owner
	for _, field := range strings.Fields(command) {
		if strings.Trim(field, `"'`) == want {
			return true
		}
	}
	return false
}

func handlerHasOwner(command string, args []string, owner string) bool {
	if commandHasOwner(command, owner) {
		return true
	}
	want := "--owner=" + owner
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// hookCommand renders the shell command a provider config invokes, including
// the argv contract (--provider, --timeout) and the in-process --filter for
// dialects that can't express the matcher.
func hookCommand(m Manifest, p agenthooks.Provider, spec HookSpec) string {
	parts := make([]string, 0, len(m.Command)+5)
	for _, c := range m.Command {
		parts = append(parts, shellQuote(c))
	}
	for _, arg := range hookArgs(m, p, spec) {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func hookArgs(m Manifest, p agenthooks.Provider, spec HookSpec) []string {
	parts := []string{"agenthooks", "run", "--provider=" + string(p), "--owner=" + manifestOwner(m)}
	if spec.Timeout > 0 {
		parts = append(parts, "--timeout="+spec.Timeout.String())
	}
	if !spec.Tools.IsEmpty() {
		if _, ok := agenthooks.CompileMatcher(p, spec.Tools); !ok {
			parts = append(parts, "--filter="+spec.Tools.Encode())
		}
	}
	return parts
}

func ownerName(m Manifest) (string, error) {
	owner := m.Owner
	if owner == "" {
		owner = "agenthooks"
	}
	if len(owner) > 64 {
		return "", errors.New("install: owner must be 1–64 ASCII letters, digits, underscores, or hyphens")
	}
	for _, r := range owner {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "", errors.New("install: owner must be 1–64 ASCII letters, digits, underscores, or hyphens")
	}
	return owner, nil
}

func manifestOwner(m Manifest) string {
	owner, err := ownerName(m)
	if err != nil {
		return ""
	}
	return owner
}

// shellQuote quotes a command argument for the shell the provider will run
// hook commands with. Configs are rendered on the machine that runs them, so
// the host OS decides the dialect: cmd.exe has no single-quote syntax, POSIX
// shells have no cmd-style escaping.
func shellQuote(s string) string {
	if runtime.GOOS == "windows" {
		if s == "" {
			return `""`
		}
		if strings.ContainsAny(s, " \t\"^&|<>()%!") {
			return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		return s
	}
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\n\"'\\$&|;<>(){}*?#~`!") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// shellQuoteBody quotes only when needed, for flag values.
func shellQuoteBody(s string) string {
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(s, " \t\"^&|<>()%!") {
			return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		return s
	}
	if strings.ContainsAny(s, " \t\n\"'\\$&|<>(){}#~`!") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

func timeoutSeconds(spec HookSpec) int {
	if spec.Timeout > 0 {
		secs := int((spec.Timeout + time.Second - 1) / time.Second)
		if secs < 1 {
			secs = 1
		}
		return secs
	}
	// SessionStart interactive flows get raised timeouts (§7).
	if spec.Kind == agenthooks.KindSessionStart {
		return 120
	}
	return 60
}

func jsonFile(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func memFS(files map[string][]byte) fs.FS {
	return memFSWithModes(files, nil)
}

func memFSWithModes(files map[string][]byte, modes map[string]fs.FileMode) fs.FS {
	out := fstest.MapFS{}
	for p, b := range files {
		mode := fs.FileMode(0o644)
		if configured, ok := modes[p]; ok {
			mode = configured.Perm()
		}
		out[p] = &fstest.MapFile{Data: b, Mode: mode}
	}
	return out
}
