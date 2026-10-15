package app

import (
	"context"
	"fmt"
	"io"
	"os"
)

const (
	ExitOK          = 0
	ExitInvalid     = 1
	ExitUsage       = ExitInvalid
	ExitConflict    = ExitInvalid
	ExitSyncChanged = 3
	ExitBlocked     = 4
)

// Run creates the default provider catalog and executes a CLI command.
func Run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	service, err := DefaultService()
	if err != nil {
		fmt.Fprintf(errOut, "style: %v\n", err)
		return ExitUsage
	}
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(errOut, "style: resolve working directory: %v\n", err)
		return ExitUsage
	}
	return RunWithService(ctx, service, workDir, args, out, errOut)
}

// RunWithService keeps command tests independent of process cwd and installed providers.
func RunWithService(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if service == nil {
		fmt.Fprintln(errOut, "style: service is required")
		return ExitUsage
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(out)
		return ExitOK
	}
	switch args[0] {
	case "init":
		return runInit(ctx, service, workDir, args[1:], out, errOut)
	case "validate":
		return runValidate(ctx, service, workDir, args[1:], out, errOut)
	case "plan":
		return runPlan(ctx, service, workDir, args[1:], out, errOut)
	case "install":
		return runInstall(ctx, service, workDir, args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "style: unknown command %q\n", args[0])
		printUsage(errOut)
		return ExitUsage
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "style init [name] [--single]")
	fmt.Fprintln(out, "style validate [path] [--json]")
	fmt.Fprintln(out, "style plan <source> [--harness auto|codex,claude-code] [--assume-requests N] [--assume-triggers N] [--json]")
	fmt.Fprintln(out, "style install <source> [--harness h] [--route h=r] [--set k=v] [--link] [--allow-replace] [--fail-closed h] [--yes] [--json]")
}
