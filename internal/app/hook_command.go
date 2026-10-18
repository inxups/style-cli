package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/inxups/style-cli/internal/probe"
	styleruntime "github.com/inxups/style-cli/internal/runtime"
	"github.com/speakeasy-api/agenthooks"
)

// Generated commands may put consumer flags before the agenthooks sentinel.
// Dispatch before constructing the management catalog: hooks load index/state,
// without inspecting package sources, lock files or other providers' config.
func isHookInvocation(args []string) bool {
	for _, arg := range args {
		if arg == "agenthooks" {
			return true
		}
		if arg != "--project-hook" && !strings.HasPrefix(arg, "--project-entry-root=") && !strings.HasPrefix(arg, "--style-home=") && !strings.HasPrefix(arg, "--harness-home=") && !strings.HasPrefix(arg, "--harness-version=") {
			return false
		}
	}
	return false
}

func runHook(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	var home, harnessHome, version string
	var provider agenthooks.Provider
	closed := false
	sentinel := -1
	for i, arg := range args {
		if arg == "agenthooks" {
			sentinel = i
			break
		}
		switch {
		case strings.HasPrefix(arg, "--style-home="):
			home = strings.TrimPrefix(arg, "--style-home=")
		case strings.HasPrefix(arg, "--harness-home="):
			harnessHome = strings.TrimPrefix(arg, "--harness-home=")
		case strings.HasPrefix(arg, "--harness-version="):
			version = strings.TrimPrefix(arg, "--harness-version=")
		}
	}
	invalid := func(message string) int { fmt.Fprintln(errOut, "style hook: "+message); return ExitUsage }
	if sentinel < 0 || sentinel+1 >= len(args) || args[sentinel+1] != "run" {
		return invalid("expected agenthooks run")
	}
	for _, arg := range args[sentinel+2:] {
		if strings.HasPrefix(arg, "--provider=") {
			value := agenthooks.Provider(strings.TrimPrefix(arg, "--provider="))
			if provider != "" && provider != value {
				return invalid("conflicting providers")
			}
			provider = value
		}
		if arg == "--fail-closed" {
			closed = true
		}
		if strings.HasPrefix(arg, "--fail-closed=") {
			value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--fail-closed="))
			if err != nil {
				return invalid("invalid fail-closed value")
			}
			closed = value
		}
	}
	var enricher adapter.RuntimeEnricher
	switch provider {
	case agenthooks.ProviderCodex:
		if closed {
			return invalid("fail-closed is supported only by claude-code")
		}
		value := codex.NewEnricher()
		value.Home, value.HarnessVersion = harnessHome, version
		enricher = value
	case agenthooks.ProviderClaudeCode:
		value := claude.NewEnricher()
		value.HarnessVersion = version
		enricher = value
	default:
		return invalid("explicit --provider=codex or --provider=claude-code is required")
	}
	projectMode := false
	for _, arg := range args[:sentinel] {
		if arg == "--project-hook" {
			projectMode = true
		}
	}
	if projectMode {
		if home != "" || harnessHome != "" || version != "" {
			return projectHookFailure(ctx, args, in, out, errOut, enricher, provider, closed, projectFailure("project entry cannot override private binding flags"))
		}
		if provider == agenthooks.ProviderClaudeCode && os.Getenv("CLAUDE_CODE_REMOTE") == "true" {
			return projectHookFailure(ctx, args, in, out, errOut, enricher, provider, closed, projectFailure("Claude cloud delivery is no longer supported; use a local Claude Code session"))
		}
		binding, err := projectRuntimeBinding(args, provider)
		if err != nil {
			return projectHookFailure(ctx, args, in, out, errOut, enricher, provider, closed, err)
		}
		h := binding.Harnesses[string(provider)]
		home = binding.Home
		harnessHome = h.ConfigDir
		version = h.Version
		if value, ok := enricher.(*codex.Enricher); ok {
			value.Home = harnessHome
			value.HarnessVersion = version
		}
		if value, ok := enricher.(*claude.Enricher); ok {
			value.HarnessVersion = version
		}
	}
	home, err := resolveStyleHome(home)
	if err != nil {
		return invalid("cannot resolve STYLE home")
	}
	runtime, err := styleruntime.Load(filepath.Join(home, "build", "index.json"), filepath.Join(home, "state"), enricher, provider, closed)
	if err != nil {
		return invalid("cannot initialize runtime")
	}
	runtime.CodecStateDir = filepath.Join(home, "cache", "agenthooks")
	pluginRoot := os.Getenv("PLUGIN_ROOT")
	if provider == agenthooks.ProviderClaudeCode {
		pluginRoot = os.Getenv("CLAUDE_PLUGIN_ROOT")
	}
	runtime.Observer = probe.NewObserver(home, harnessHome, string(provider), pluginRoot)
	return runtime.Run(ctx, args, in, out, errOut)
}
