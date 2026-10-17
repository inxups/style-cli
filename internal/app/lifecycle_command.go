package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/inxups/style-cli/internal/source"
)

func runInstall(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	if isHelp(args) {
		fmt.Fprintln(out, "style install <source> [--ref ref] [--path repo-path] [--harness h] [--route h=r] [--set k=v] [--link] [--allow-replace] [--fail-closed h] [--yes] [--json]")
		return ExitOK
	}
	request, jsonOutput, err := parseInstallArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style install: %v\n", err)
		return ExitUsage
	}
	if request.Source == "" {
		fmt.Fprintln(errOut, "style install: source is required")
		return ExitUsage
	}
	if !source.Remote(request.Source) && !filepath.IsAbs(request.Source) {
		request.Source = filepath.Join(workDir, request.Source)
	}
	previewWritten := false
	if !jsonOutput {
		yes := request.Yes
		request.Yes = false
		request.Confirm = func(preview InstallResult) (bool, error) {
			writeInstallPreview(out, preview)
			previewWritten = true
			return yes, nil
		}
	}
	result, err := service.Install(ctx, request)
	if err != nil {
		if result.AwaitingConfirmation && jsonOutput {
			_ = encodeJSON(out, result)
		}
		var usageErr *UsageError
		var conflictErr *ConflictError
		var blockedErr *BlockedError
		var validationErr *ValidationError
		switch {
		case errors.As(err, &usageErr), errors.As(err, &conflictErr):
			fmt.Fprintf(errOut, "style install: %v\n", err)
			return ExitInvalid
		case errors.As(err, &blockedErr):
			fmt.Fprintf(errOut, "style install: %v\n", err)
			return ExitBlocked
		case errors.As(err, &validationErr):
			if jsonOutput {
				_ = encodeJSON(out, validationErr.Diagnostics)
			} else {
				writeDiagnostics(out, validationErr.Diagnostics)
			}
			return ExitInvalid
		default:
			fmt.Fprintf(errOut, "style install: %v\n", err)
			return ExitInvalid
		}
	}
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style install: encode report: %v\n", err)
			return ExitInvalid
		}
		return ExitOK
	}
	if !previewWritten {
		writeInstallPreview(out, result)
	}
	if len(result.Reports) == 0 && len(result.Routes) == 0 && len(result.Diagnostics) > 0 && !result.Changed {
		fmt.Fprintf(out, "skipped %s %s (no compatible harness)\n", result.Package, result.Version)
	} else {
		fmt.Fprintf(out, "installed %s %s\n", result.Package, result.Version)
	}
	if result.Replace {
		fmt.Fprintln(out, "warning: replace mode is active")
	}
	if result.Link {
		fmt.Fprintln(out, "source: link")
	}
	for _, route := range result.Routes {
		fmt.Fprintf(out, "route: %s\n", route)
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(out, "warning %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
	return ExitOK
}

func writeInstallPreview(out io.Writer, result InstallResult) {
	writeSource(out, result.Source)
	if len(result.Reports) > 0 {
		writePlanText(out, PlanResult{Package: result.Package, Version: result.Version, Diagnostics: result.Diagnostics, Reports: result.Reports})
	}
	writeStaticMigrations(out, result.StaticMigrations)
	writeNativeHooks(out, result.Native)
}

func parseInstallArgs(args []string) (InstallRequest, bool, error) {
	request := InstallRequest{Settings: map[string]string{}, Routes: map[string]string{}}
	jsonOutput := false
	for index := 0; index < len(args); index++ {
		if handled, err := parseSourceOption(args, &index, &request.Ref, &request.Path); handled {
			if err != nil {
				return InstallRequest{}, false, err
			}
			continue
		}
		arg := args[index]
		switch {
		case arg == "--json":
			jsonOutput = true
		case arg == "--yes":
			request.Yes = true
		case arg == "--link":
			request.Link = true
		case arg == "--allow-replace":
			request.AllowReplace = true
		case arg == "--harness":
			if index+1 >= len(args) {
				return InstallRequest{}, false, errors.New("--harness requires a value")
			}
			request.Harnesses = append(request.Harnesses, args[index+1])
			index++
		case strings.HasPrefix(arg, "--harness="):
			request.Harnesses = append(request.Harnesses, strings.TrimPrefix(arg, "--harness="))
		case arg == "--route":
			if index+1 >= len(args) {
				return InstallRequest{}, false, errors.New("--route requires harness=route")
			}
			if err := parseRoute(request.Routes, args[index+1]); err != nil {
				return InstallRequest{}, false, err
			}
			index++
		case strings.HasPrefix(arg, "--route="):
			if err := parseRoute(request.Routes, strings.TrimPrefix(arg, "--route=")); err != nil {
				return InstallRequest{}, false, err
			}
		case arg == "--set":
			if index+1 >= len(args) {
				return InstallRequest{}, false, errors.New("--set requires name=value")
			}
			if err := parseSetting(request.Settings, args[index+1]); err != nil {
				return InstallRequest{}, false, err
			}
			index++
		case strings.HasPrefix(arg, "--set="):
			if err := parseSetting(request.Settings, strings.TrimPrefix(arg, "--set=")); err != nil {
				return InstallRequest{}, false, err
			}
		case arg == "--fail-closed":
			if index+1 >= len(args) {
				return InstallRequest{}, false, errors.New("--fail-closed requires a harness")
			}
			request.FailClosed = append(request.FailClosed, args[index+1])
			index++
		case strings.HasPrefix(arg, "--fail-closed="):
			request.FailClosed = append(request.FailClosed, strings.TrimPrefix(arg, "--fail-closed="))
		case arg == "--help", arg == "-h":
			return InstallRequest{}, false, errors.New("usage: style install <source> [options]")
		default:
			if strings.HasPrefix(arg, "-") {
				return InstallRequest{}, false, fmt.Errorf("unknown option %q", arg)
			}
			if request.Source != "" {
				return InstallRequest{}, false, errors.New("only one source path is allowed")
			}
			request.Source = arg
		}
	}
	return request, jsonOutput, nil
}

func parseRoute(routes map[string]string, value string) error {
	harness, route, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(harness) == "" || strings.TrimSpace(route) == "" {
		return errors.New("--route requires harness=route")
	}
	routes[strings.ToLower(strings.TrimSpace(harness))] = strings.TrimSpace(route)
	return nil
}

func parseSetting(settings map[string]string, value string) error {
	name, setting, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return errors.New("--set requires name=value")
	}
	settings[strings.TrimSpace(name)] = setting
	return nil
}

func runUninstall(ctx context.Context, service *Service, args []string, out, errOut io.Writer) int {
	if isHelp(args) {
		fmt.Fprintln(out, "style uninstall <name|--all> [--yes] [--json]")
		return ExitOK
	}
	request, jsonOutput, err := parseUninstallArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style uninstall: %v\n", err)
		return ExitUsage
	}
	result, err := service.Uninstall(ctx, request)
	if err != nil {
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			fmt.Fprintf(errOut, "style uninstall: %v\n", err)
			return ExitInvalid
		}
		fmt.Fprintf(errOut, "style uninstall: %v\n", err)
		return ExitInvalid
	}
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style uninstall: encode report: %v\n", err)
			return ExitInvalid
		}
		return ExitOK
	}
	for _, name := range result.Removed {
		fmt.Fprintf(out, "uninstalled %s\n", name)
	}
	writeStaticMigrations(out, result.StaticMigrations)
	for _, route := range result.Routes {
		fmt.Fprintf(out, "route updated: %s\n", route)
	}
	return ExitOK
}

func parseUninstallArgs(args []string) (UninstallRequest, bool, error) {
	request := UninstallRequest{}
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--yes":
			request.Yes = true
		case "--all":
			request.All = true
		case "--json":
			jsonOutput = true
		case "--help", "-h":
			return UninstallRequest{}, false, errors.New("usage: style uninstall <name|--all> [--yes] [--json]")
		default:
			if strings.HasPrefix(arg, "-") {
				return UninstallRequest{}, false, fmt.Errorf("unknown option %q", arg)
			}
			if request.Name != "" {
				return UninstallRequest{}, false, errors.New("only one package name is allowed")
			}
			request.Name = arg
		}
	}
	if request.Name == "" && !request.All {
		return UninstallRequest{}, false, errors.New("package name or --all is required")
	}
	if request.Name != "" && request.All {
		return UninstallRequest{}, false, errors.New("package name cannot be combined with --all")
	}
	return request, jsonOutput, nil
}
