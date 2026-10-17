package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
)

type EffortReport struct {
	MappingSHA256 string `json:"mapping_sha256,omitempty"`
	MappingSource string `json:"mapping_source,omitempty"`
	Provider      string `json:"provider"`
	Source        string `json:"source"`
	Raw           string `json:"raw"`
	Tier          string `json:"tier"`
	Approximate   *bool  `json:"approximate"`
	Detail        string `json:"detail"`
}

func inspectEffort(report AdapterReport, workDir string) (EffortReport, error) {
	result := EffortReport{Provider: report.Provider, Source: "unknown"}
	switch report.Provider {
	case "codex":
		value, err := codex.ResolveEffort(codex.EffortInput{Home: report.ConfigDir, CWD: workDir, HarnessVersion: report.Version})
		if err != nil {
			return result, err
		}
		result.Raw, result.Tier, result.Approximate = value.EffortRaw, value.Effort, value.EffortApprox
		if value.Source != "" {
			result.Source = value.Source
		}
		result.Detail = "configuration fallback only; launch overrides/session values are available in hooks; project layer skipped without a trust decision"
	case "claude-code":
		value, err := claude.ResolveCurrentEffort("", state.Session{})
		if err != nil {
			return result, err
		}
		result.Raw, result.Tier, result.Approximate = value.EffortRaw, value.Effort, value.EffortApprox
		if value.Source != "" {
			result.Source = value.Source
		}
		result.Detail = "environment fallback only; event/session values are available in hooks"
	default:
		result.Detail = "adapter does not expose a diagnostic effort resolver"
	}
	return result, nil
}

type countedOutput struct{ bytes int64 }

func (output *countedOutput) Write(data []byte) (int, error) {
	output.bytes += int64(len(data))
	return len(data), nil
}

func checkLoginShell(ctx context.Context, shell, workDir string) error {
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, shell, "-lc", "true")
	command.Dir = workDir
	command.WaitDelay = 200 * time.Millisecond
	output := &countedOutput{}
	command.Stdout, command.Stderr = output, io.Discard
	if err := command.Run(); err != nil {
		return fmt.Errorf("login shell true failed: %w", err)
	}
	if output.bytes > 0 {
		return fmt.Errorf("login shell true wrote %d stdout bytes; move startup output to interactive-shell configuration", output.bytes)
	}
	return nil
}

func checkStateRoot(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("state root must be a real directory")
	}
	return nil
}

func maintainState(root string, reset bool) (int, error) {
	if err := checkStateRoot(root); err != nil {
		return 0, err
	}
	if reset {
		return 0, os.RemoveAll(root)
	}
	store, err := state.New(root)
	if err != nil {
		return 0, err
	}
	return store.Cleanup(state.DefaultExpiry)
}
