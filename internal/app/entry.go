package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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
	return RunWithInput(ctx, args, os.Stdin, out, errOut)
}

// RunWithInput is the stream-oriented process entry point, including hooks.
func RunWithInput(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if isHookInvocation(args) {
		return runHook(ctx, args, in, out, errOut)
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
	return RunWithServiceInput(ctx, service, workDir, args, in, out, errOut)
}

// RunWithService keeps command tests independent of process cwd and installed
// providers. Without input every confirmation prompt is declined.
func RunWithService(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	return RunWithServiceInput(ctx, service, workDir, args, nil, out, errOut)
}

// RunWithServiceInput answers confirmation prompts from in.
func RunWithServiceInput(ctx context.Context, service *Service, workDir string, args []string, in io.Reader, out, errOut io.Writer) int {
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
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" && len(args) == 1 {
		printUsage(out)
		return ExitOK
	}
	if args[0] == "help" {
		if _, ok := lookupCommand(args[1]); !ok || len(args) > 2 {
			fmt.Fprintf(errOut, "style help: unknown command %q\n", strings.Join(args[1:], " "))
			printUsage(errOut)
			return ExitUsage
		}
		printCommandHelp(out, args[1])
		return ExitOK
	}
	if _, ok := lookupCommand(args[0]); ok && helpRequested(args[1:]) {
		printCommandHelp(out, args[0])
		return ExitOK
	}
	if projectInvocation(args) {
		return runProjectCommand(ctx, service, workDir, args, in, out, errOut)
	}
	switch args[0] {
	case "init":
		return runInit(ctx, service, workDir, args[1:], out, errOut)
	case "validate":
		return runValidate(ctx, service, workDir, args[1:], out, errOut)
	case "count":
		return runCount(ctx, service, workDir, args[1:], out, errOut)
	case "plan":
		return runPlan(ctx, service, workDir, args[1:], out, errOut)
	case "install":
		return runInstall(ctx, service, workDir, args[1:], in, out, errOut)
	case "update":
		return runUpdate(ctx, service, args[1:], in, out, errOut)
	case "restore":
		return runRestore(ctx, service, args[1:], in, out, errOut)
	case "uninstall":
		return runUninstall(ctx, service, args[1:], in, out, errOut)
	case "sync", "trust":
		return runSync(ctx, service, args[0], args[1:], in, out, errOut)
	case "list", "doctor":
		return runInspect(ctx, service, workDir, args[0], args[1:], out, errOut)
	case "probe":
		return runProbe(ctx, service, args[1:], out, errOut)
	case "explain":
		return runExplain(ctx, service, workDir, args[1:], out, errOut)
	case "stats":
		return runScopedStats(ctx, service, args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "style: unknown command %q\n", args[0])
		printUsage(errOut)
		return ExitUsage
	}
}

func printCommandHelp(out io.Writer, name string) {
	command, _ := lookupCommand(name)
	fmt.Fprintln(out, command.usage())
}
