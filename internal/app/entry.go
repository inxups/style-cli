package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	ExitOK      = 0
	ExitInvalid = 1
	// ExitUsage reports a command line that could not be parsed into a
	// request. Requests the service rejects use ExitInvalid.
	ExitUsage       = 2
	ExitConflict    = ExitInvalid
	ExitSyncChanged = 3
	ExitBlocked     = 4
)

// Version is the CLI release; release builds may set it with
// -ldflags "-X github.com/inxups/style-cli/internal/app.Version=…".
var Version = "0.3.0"

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
	if len(args) > 0 && args[0] == "--version" {
		// The version needs no harness detection.
		return runVersion(args[1:], out, errOut)
	}
	service, err := DefaultService()
	if err != nil {
		fmt.Fprintf(errOut, "style: %v\n", err)
		return ExitInvalid
	}
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(errOut, "style: resolve working directory: %v\n", err)
		return ExitInvalid
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
		return ExitInvalid
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" && len(args) == 1 {
		printUsage(out)
		return ExitOK
	}
	if args[0] == "--version" {
		return runVersion(args[1:], out, errOut)
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
	if jsonRequested(args) {
		return runJSON(args[0], service.jsonScope(), out, errOut, func(out, errOut io.Writer) int {
			return runCommand(ctx, service, workDir, args, in, out, errOut)
		})
	}
	return runCommand(ctx, service, workDir, args, in, out, errOut)
}

func runVersion(args []string, out, errOut io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintf(errOut, "style: --version takes no arguments, got %q\n", strings.Join(args, " "))
		return ExitUsage
	}
	fmt.Fprintf(out, "style %s\n", Version)
	return ExitOK
}

func runCommand(ctx context.Context, service *Service, workDir string, args []string, in io.Reader, out, errOut io.Writer) int {
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
