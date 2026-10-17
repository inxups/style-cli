package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
	"github.com/speakeasy-api/agenthooks/install"
)

// Discovery uses process cwd, never the untrusted event payload. A copied
// declaration alone grants no runtime access to a private installation.
func projectRuntimeBinding(args []string, provider agenthooks.Provider) (*projectBinding, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	canonical, err := canonicalProjectPath(cwd)
	if err != nil {
		return nil, err
	}
	if provider == agenthooks.ProviderCodex && filepath.Clean(cwd) != canonical {
		return nil, projectFailure("start Codex using the canonical project path")
	}
	var root string
	for dir, depth := canonical, 0; ; dir, depth = filepath.Dir(dir), depth+1 {
		if depth > 128 {
			return nil, projectFailure("project discovery exceeded directory limit")
		}
		if _, e := os.Lstat(filepath.Join(dir, projectDeclarationPath)); e == nil {
			if root != "" {
				return nil, projectFailure("ambiguous nested project declarations")
			}
			root = dir
		} else if !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	if root == "" {
		return nil, projectFailure("no project declaration found from process cwd")
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--project-entry-root=") {
			entry, e := canonicalProjectPath(strings.TrimPrefix(arg, "--project-entry-root="))
			if e != nil || entry != root {
				return nil, projectFailure("project launcher root differs from process cwd")
			}
		}
	}
	if _, err = readProjectDeclaration(root); err != nil {
		return nil, err
	}
	base, err := resolveStyleHome("")
	if err != nil {
		return nil, err
	}
	base, err = canonicalFuturePath(base)
	if err != nil {
		return nil, err
	}
	home := projectHome(base, root)
	path := filepath.Join(home, "binding.json")
	if err = install.CheckPath(home, path); err != nil {
		return nil, err
	}
	data, err := saferead.ReadFile(path, 1<<20)
	if err != nil {
		return nil, projectFailure("project has no local binding; install explicitly at this root")
	}
	var binding projectBinding
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if dec.Decode(&binding) != nil || dec.Decode(new(any)) != io.EOF {
		return nil, projectFailure("invalid runtime binding")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = canonicalProjectPath(exe)
	if err != nil {
		return nil, err
	}
	declaration, err := saferead.ReadFile(filepath.Join(root, projectDeclarationPath), 1<<20)
	if err != nil || hashBytes(declaration) != binding.Declaration {
		return nil, projectFailure("project declaration differs from approved runtime binding")
	}
	h, ok := binding.Harnesses[string(provider)]
	if binding.Scope != "project" || binding.Root != root || binding.Base != base || binding.Home != home || binding.EntryVersion != projectEntryVersion || binding.Runner != exe || !ok || h.ConfigDir == "" || h.Version == "" {
		return nil, projectFailure("runtime identity differs from approved project binding")
	}
	expectedRoute := "codex-project"
	if provider == agenthooks.ProviderClaudeCode {
		expectedRoute = "claude-project"
	}
	envName, sub := "CODEX_HOME", ".codex"
	if provider == agenthooks.ProviderClaudeCode {
		envName, sub = "CLAUDE_CONFIG_DIR", ".claude"
	}
	actual := os.Getenv(envName)
	if actual == "" {
		userHome, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		actual = filepath.Join(userHome, sub)
	}
	actual, e := canonicalFuturePath(actual)
	if e != nil {
		return nil, e
	}
	expected, e := canonicalFuturePath(h.ConfigDir)
	if e != nil || actual != expected {
		return nil, projectFailure("harness user configuration differs from the approved binding")
	}
	if h.Route != expectedRoute {
		return nil, projectFailure("runtime route differs")
	}
	return &binding, nil
}
func projectHookFailure(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, enricher adapter.RuntimeEnricher, provider agenthooks.Provider, closed bool, cause error) int {
	fmt.Fprintln(errOut, "style project hook: "+cause.Error())
	store, err := state.New(filepath.Join(os.TempDir(), "style-project-failed-no-state"))
	if err != nil {
		return ExitInvalid
	}
	runner, err := styleruntime.New(nil, store, enricher, provider, closed)
	if err != nil {
		return ExitInvalid
	}
	runner.InitializationError = cause
	return runner.Run(ctx, args, in, out, errOut, agenthooks.WithoutMCPResolution(), agenthooks.WithoutDedup())
}
