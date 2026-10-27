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

// runInstall runs install, and link, which installs a local directory as a
// development link.
func runInstall(ctx context.Context, service *Service, workDir, command string, args []string, in io.Reader, out, errOut io.Writer) int {
	spec, _ := lookupCommand(command)
	request, jsonOutput, err := parseInstallArgs(spec, args)
	if err == nil {
		_, err = service.parseHarnesses(request.Harnesses)
	}
	if err != nil {
		return spec.usageError(errOut, err)
	}
	if request.Source == "" {
		return spec.usageError(errOut, fmt.Errorf("source is required"))
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
		question := "Install with these capabilities?"
		if request.Link {
			question = "Link with these capabilities?"
		}
		return confirmInput(in, errOut, question), nil
	}
	result, err := service.Install(ctx, request)
	if err != nil {
		if result.AwaitingConfirmation {
			if jsonOutput {
				result.Diagnostics = append(result.Diagnostics, declinedDiagnostic(command))
				_ = encodeJSON(out, result)
			} else {
				writeCommandDiagnostics(errOut, command, []Diagnostic{declinedDiagnostic(command)})
			}
			return ExitInvalid
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			if jsonOutput {
				_ = encodeJSON(out, validationErr.Diagnostics)
			} else {
				writeCommandDiagnostics(errOut, command, validationErr.Diagnostics)
			}
			return ExitInvalid
		}
		commandFailure(out, errOut, jsonOutput, command, err)
		return commandExit(err)
	}
	result.Diagnostics = append(result.Diagnostics, registrationDiagnostics(result.Registration)...)
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style %s: encode report: %v\n", command, err)
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
		fmt.Fprintf(out, "%s %s %s\n", map[bool]string{false: "installed", true: "linked"}[result.Link], result.Package, result.Version)
	}
	if result.Replace {
		fmt.Fprintln(out, "warning: replace mode is active")
	}
	for _, route := range result.Routes {
		fmt.Fprintf(out, "route: %s\n", route)
	}
	writeCommandDiagnostics(errOut, command, result.Diagnostics)
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

func parseInstallArgs(spec commandSpec, args []string) (InstallRequest, bool, error) {
	parsed, err := spec.parse(args)
	if err != nil {
		return InstallRequest{}, false, err
	}
	request := InstallRequest{
		Source: parsed.Arg(0), Harnesses: parsed.List("--harness"),
		Link: spec.Name == "link", AllowReplace: parsed.Bool("--allow-replace"),
		Settings: map[string]string{}, Routes: map[string]string{}, ProjectSettings: map[string]string{},
	}
	if len(parsed.Positional) > 1 {
		request.ProjectSources = parsed.Positional[1:]
	}
	for _, value := range parsed.Positional {
		if err := sourceArgument(value); err != nil {
			return InstallRequest{}, false, err
		}
	}
	if request.Link && source.Remote(request.Source) {
		return InstallRequest{}, false, errors.New("source-invalid: link needs a local directory; Git sources cannot be linked")
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
	for _, value := range parsed.List("--set") {
		settings := request.Settings
		if len(request.ProjectSources) > 0 {
			// With several sources every setting names its package.
			if pkg, _, ok := strings.Cut(value, "."); !ok || pkg == "" || strings.Contains(pkg, "=") {
				return InstallRequest{}, false, errors.New("with several sources, --set needs package.key=value")
			}
			settings = request.ProjectSettings
		}
		if err := parseSetting(settings, value); err != nil {
			return InstallRequest{}, false, err
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
	request.Confirm = func(preview UninstallResult) (bool, error) {
		previewOut := out
		if jsonOutput {
			previewOut = errOut
		}
		writeUninstallPreview(previewOut, preview)
		return confirmInput(in, errOut, question), nil
	}
	result, err := service.Uninstall(ctx, request)
	if err != nil {
		if result.AwaitingConfirmation {
			if jsonOutput {
				result.Diagnostics = append(result.Diagnostics, declinedDiagnostic("uninstall"))
				_ = encodeJSON(out, result)
			} else {
				writeCommandDiagnostics(errOut, "uninstall", []Diagnostic{declinedDiagnostic("uninstall")})
			}
			return ExitInvalid
		}
		commandFailure(out, errOut, jsonOutput, "uninstall", err)
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
	if len(result.Removed) == 0 && result.Changed {
		fmt.Fprintln(out, "cleaned residual STYLE installation")
	}
	writeStaticMigrations(out, result.StaticMigrations)
	for _, route := range result.Routes {
		fmt.Fprintf(out, "route updated: %s\n", route)
	}
	return ExitOK
}

func writeUninstallPreview(out io.Writer, preview UninstallResult) {
	if len(preview.Removed) == 0 {
		fmt.Fprintln(out, "Uninstall: clean residual STYLE installation")
	} else {
		fmt.Fprintf(out, "Uninstall: remove %s\n", strings.Join(preview.Removed, ", "))
	}
	if len(preview.Remaining) > 0 {
		fmt.Fprintf(out, "remaining: %s\n", strings.Join(preview.Remaining, ", "))
	}
	for _, route := range preview.Routes {
		fmt.Fprintf(out, "route: %s\n", route)
	}
	writeStaticMigrations(out, preview.StaticMigrations)
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
