package agenthooks

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"
)

// CodexLaunchSettings exposes only the launch facts STYLE needs to interpret
// Codex effort and model-service configuration. Arbitrary -c values remain private because they
// may contain unrelated or sensitive settings.
type CodexLaunchSettings struct {
	Profile              string
	ModelReasoningEffort string
	ModelProvider        string
	ModelProviderSet     bool
	IgnoreUserConfig     bool
}

// CurrentCodexLaunchSettings recovers the invoking Codex command from the
// ancestor process list. The boolean is false when no Codex ancestor is found.
func CurrentCodexLaunchSettings(cwd string) (CodexLaunchSettings, bool) {
	launch, ok := currentCodexLaunchContext(cwd)
	if !ok {
		return CodexLaunchSettings{}, false
	}
	return launch.styleSettings(), true
}

type codexLaunchContext struct {
	CWD          string   `json:"cwd"`
	Executable   string   `json:"executable,omitempty"`
	Profile      string   `json:"profile,omitempty"`
	Overrides    []string `json:"overrides,omitempty"`
	Unreplayable bool     `json:"unreplayable,omitempty"`
}

func currentCodexLaunchContext(cwd string) (codexLaunchContext, bool) {
	args, pid := findAncestorProcess(isCodexProcessArgs)
	if args == nil {
		return codexLaunchContext{}, false
	}
	ctx := parseCodexLaunchArgs(args, cwd)
	if executable, err := procExecutable(pid); err == nil && executable != "" {
		ctx.Executable = executable
	}
	return ctx, true
}

func isCodexProcessArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0])))
	return name == "codex" || strings.HasPrefix(name, "codex-")
}

func parseCodexLaunchArgs(args []string, cwd string) codexLaunchContext {
	ctx := codexLaunchContext{CWD: cwd}
	if len(args) > 0 {
		ctx.Executable = args[0]
		args = args[1:]
	}
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			break
		}
		switch arg {
		case "-c", "--config", "--enable", "--disable":
			if len(args) > 0 {
				ctx.Overrides = append(ctx.Overrides, canonicalCodexFlag(arg), args[0])
				args = args[1:]
			}
		case "-p", "--profile":
			if len(args) > 0 {
				ctx.Profile = args[0]
				args = args[1:]
			}
		case "--ignore-user-config":
			ctx.Unreplayable = true
		default:
			name, value, ok := strings.Cut(arg, "=")
			if ok {
				switch name {
				case "--config", "--enable", "--disable":
					ctx.Overrides = append(ctx.Overrides, name, value)
					continue
				case "--profile":
					ctx.Profile = value
					continue
				}
			}
			switch {
			case strings.HasPrefix(arg, "-c") && len(arg) > 2:
				ctx.Overrides = append(ctx.Overrides, "--config", arg[2:])
			case strings.HasPrefix(arg, "-p") && len(arg) > 2:
				ctx.Profile = arg[2:]
			}
		}
	}
	return ctx
}

func canonicalCodexFlag(flag string) string {
	switch flag {
	case "-c":
		return "--config"
	case "-p":
		return "--profile"
	default:
		return flag
	}
}

func (c codexLaunchContext) replayArgs() []string {
	args := make([]string, 0, len(c.Overrides)+2)
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	return append(args, c.Overrides...)
}

func (c codexLaunchContext) hasOverrides() bool {
	return c.Profile != "" || len(c.Overrides) > 0 || c.Unreplayable
}

func (c codexLaunchContext) styleSettings() CodexLaunchSettings {
	settings := CodexLaunchSettings{Profile: c.Profile, IgnoreUserConfig: c.Unreplayable}
	for i := 0; i+1 < len(c.Overrides); i += 2 {
		if c.Overrides[i] != "--config" {
			continue
		}
		key, value, ok := strings.Cut(c.Overrides[i+1], "=")
		if ok && strings.TrimSpace(key) == "model_provider" {
			settings.ModelProvider = unquoteCodexValue(strings.TrimSpace(value))
			settings.ModelProviderSet = true
		}
		if ok && strings.TrimSpace(key) == "model_reasoning_effort" {
			settings.ModelReasoningEffort = unquoteCodexValue(strings.TrimSpace(value))
		}
	}
	return settings
}

func unquoteCodexValue(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

func (c codexLaunchContext) cacheKey() string {
	h := sha256.New()
	writeHashPart := func(value string) {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	writeHashPart("agenthooks-codex-mcp-v1")
	writeHashPart(c.CWD)
	writeHashPart(c.Executable)
	writeHashPart(c.Profile)
	writeHashPart(strconv.FormatBool(c.Unreplayable))
	for _, arg := range c.Overrides {
		writeHashPart(arg)
	}
	hashLaunchEnvironment(writeHashPart)
	return hex.EncodeToString(h.Sum(nil))[:32]
}
