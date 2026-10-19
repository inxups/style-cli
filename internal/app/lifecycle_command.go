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

func runInstall(ctx context.Context, service *Service, workDir string, args []string, in io.Reader, out, errOut io.Writer) int {
	request, jsonOutput, err := parseInstallArgs(args)
	if err == nil {
		_, err = service.parseHarnesses(request.Harnesses)
	}
	if err != nil {
		return installCommand.usageError(errOut, err)
	}
	if request.Source == "" {
		fmt.Fprintln(errOut, "style install: source is required")
		return ExitUsage
	}
	if !source.Remote(request.Source) && !filepath.IsAbs(request.Source) {
		request.Source = filepath.Join(workDir, request.Source)
	}
	for i, path := range request.ProjectSources {
		if !source.Remote(path) && !filepath.IsAbs(path) {
			request.ProjectSources[i] = filepath.Join(workDir, path)
		}
	}
	previewWritten := false
	request.Confirm = func(preview InstallResult) (bool, error) {
		previewOut := out
		if jsonOutput {
			previewOut = errOut
		}
		writeInstallPreview(previewOut, preview)
		previewWritten = !jsonOutput
		return confirmInput(in, errOut, "Install with these capabilities?"), nil
	}
	result, err := service.Install(ctx, request)
	if err != nil {
		if result.AwaitingConfirmation {
			if jsonOutput {
				_ = encodeJSON(out, result)
			} else {
				fmt.Fprintln(errOut, "install canceled; nothing was changed")
			}
			return ExitInvalid
		}
		var usageErr *UsageError
		var conflictErr *ConflictError
		var blockedErr *BlockedError
		var validationErr *ValidationError
		switch {
		case errors.As(err, &usageErr):
			fmt.Fprintf(errOut, "style install: %v\n", err)
			return ExitUsage
		case errors.As(err, &conflictErr):
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
		if registrationRequired(result.Registration) {
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
	if registrationRequired(result.Registration) {
		return ExitInvalid
	}
	return ExitOK
}

func writeInstallPreview(out io.Writer, result InstallResult) {
	for _, registration := range result.Registration {
		fmt.Fprintf(out, "plugin registration: %s (generation is not delivery)\n", registration.State)
		for _, command := range registration.Commands {
			fmt.Fprintln(out, command)
		}
	}
	if result.ProjectAdoption {
		fmt.Fprintln(out, "project adoption: existing declared STYLE effects become owned by this clone and are removed on uninstall")
	}
	for _, pkg := range result.ProjectPackages {
		fmt.Fprintf(out, "project package: %s %s source=%s\n", pkg.Name, pkg.Version, pkg.Source)
		writeSource(out, pkg.Git)
	}
	writeSource(out, result.Source)
	if len(result.Reports) > 0 {
		writePlanText(out, PlanResult{Package: result.Package, Version: result.Version, Diagnostics: result.Diagnostics, Reports: result.Reports})
	}
	writeStaticMigrations(out, result.StaticMigrations)
	writeNativeHooks(out, result.Native)
}

func parseInstallArgs(args []string) (InstallRequest, bool, error) {
	parsed, err := installCommand.parse(args)
	if err != nil {
		return InstallRequest{}, false, err
	}
	request := InstallRequest{
		Source:    parsed.Arg(0),
		Harnesses: parsed.List("--harness"), ProjectSources: parsed.List("--project-source"),
		Link: parsed.Bool("--link"), AllowReplace: parsed.Bool("--allow-replace"),
		Settings: map[string]string{}, Routes: map[string]string{}, ProjectSettings: map[string]string{},
	}
	if parsed.Bool("--fail-closed") {
		// Fail-closed is a Claude Code installation mode; it also selects that harness.
		request.FailClosed = []string{"claude-code"}
	}
	for _, value := range parsed.List("--route") {
		if err := parseRoute(request.Routes, value); err != nil {
			return InstallRequest{}, false, err
		}
	}
	for _, option := range []struct {
		name     string
		settings map[string]string
	}{{"--set", request.Settings}, {"--project-set", request.ProjectSettings}} {
		for _, value := range parsed.List(option.name) {
			if err := parseSetting(option.settings, value); err != nil {
				return InstallRequest{}, false, err
			}
		}
	}
	return request, parsed.Bool("--json"), nil
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

func runUninstall(ctx context.Context, service *Service, args []string, in io.Reader, out, errOut io.Writer) int {
	request, jsonOutput, err := parseUninstallArgs(args)
	if err != nil {
		return uninstallCommand.usageError(errOut, err)
	}
	question := "Uninstall " + request.Name + "?"
	if request.All {
		question = "Uninstall all packages?"
	}
	if request.Yes = confirmInput(in, errOut, question); !request.Yes {
		fmt.Fprintln(errOut, "uninstall canceled; nothing was changed")
		return ExitInvalid
	}
	result, err := service.Uninstall(ctx, request)
	if err != nil {
		fmt.Fprintf(errOut, "style uninstall: %v\n", err)
		return commandExit(err)
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
	parsed, err := uninstallCommand.parse(args)
	if err != nil {
		return UninstallRequest{}, false, err
	}
	request := UninstallRequest{Name: parsed.Arg(0), All: parsed.Bool("--all")}
	if request.Name == "" && !request.All {
		return UninstallRequest{}, false, errors.New("package name or --all is required")
	}
	if request.Name != "" && request.All {
		return UninstallRequest{}, false, errors.New("package name cannot be combined with --all")
	}
	return request, parsed.Bool("--json"), nil
}
