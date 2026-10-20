package app

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runInspect(ctx context.Context, service *Service, workDir, command string, args []string, out, errOut io.Writer) int {
	spec, _ := lookupCommand(command)
	parsed, err := spec.parse(args)
	if err != nil {
		return spec.usageError(errOut, err)
	}
	jsonOutput, reset := parsed.Bool("--json"), parsed.Bool("--reset-state")
	var value any
	exit := ExitOK
	switch command {
	case "list":
		value, err = service.List(ctx, ListRequest{})
	case "doctor":
		var result DoctorResult
		result, err = service.Doctor(ctx, DoctorRequest{WorkDir: workDir, ResetState: reset})
		value = result
		if !result.Healthy {
			exit = ExitInvalid
		}
	}
	if err != nil {
		commandFailure(out, errOut, jsonOutput, command, err)
		return commandExit(err)
	}
	if jsonOutput {
		if err := encodeJSON(out, value); err != nil {
			fmt.Fprintf(errOut, "style %s: encode report: %v\n", command, err)
			return ExitInvalid
		}
		return exit
	}
	switch result := value.(type) {
	case ListResult:
		writeListText(out, result.Packages)
	case DoctorResult:
		fmt.Fprintf(out, "healthy=%t\n", result.Healthy)
		writeListText(out, result.Packages)
		writeAdaptersText(out, result.Adapters)
		for _, native := range result.Native {
			fmt.Fprintf(out, "native %s %s: %s; event=%s matcher=%q timeout=%ds command=%s\n", native.Harness, native.Rule,
				native.Status, native.Event, native.Matcher, native.Timeout, native.Command)
			if native.TrustStatus != "" {
				fmt.Fprintf(out, "  trusted_hash: %s\n", native.TrustStatus)
			}
		}
		for _, effort := range result.Effort {
			fmt.Fprintf(out, "effort %s: source=%s raw=%q tier=%q; %s\n", effort.Provider, effort.Source, effort.Raw, effort.Tier, effort.Detail)
		}
		fmt.Fprintln(out, "Top rules (last 7 days; estimated tokens; calibration pending):")
		for _, cost := range result.TopRules {
			fmt.Fprintf(out, "  %s %s: %d tokens, %d emits, %d body hashes\n", cost.Provider, cost.Rule, cost.EstimatedTokens, cost.Emits, cost.DistinctBodyHashes)
		}
		fmt.Fprintf(out, "state: expired=%d reset=%t; legacy stats without estimate=%d\n", result.ExpiredSessions, result.StateReset, result.LegacyStats)
		writeDiagnostics(out, result.Diagnostics)
		if !result.Healthy {
			// The report stays on stdout; stderr says why the exit code is 1.
			summary := Diagnostic{Code: "unhealthy", Severity: "error", Message: "the installation is not healthy"}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Severity == "error" {
					summary = diagnostic
					break
				}
			}
			writeCommandDiagnostics(errOut, "doctor", []Diagnostic{summary})
		}
	}
	return exit
}

func writeListText(out io.Writer, packages []InstalledPackage) {
	if len(packages) == 0 {
		fmt.Fprintln(out, "No packages installed.")
	}
	for _, pkg := range packages {
		fmt.Fprintf(out, "%s %s seq=%d source=%s:%s link=%s hash_matches=%t native=%d\n", pkg.Name, pkg.Version, pkg.Sequence, pkg.SourceKind, pkg.Source, pkg.Link, pkg.HashMatches, pkg.NativeCount)
		if pkg.Git != nil {
			fmt.Fprintf(out, "  git: %q ref=%q commit=%s object=%s:%s content=%s\n", pkg.Git.URL, pkg.Git.Ref, pkg.Git.Commit, pkg.Git.ObjectFormat, pkg.Git.Object, pkg.Git.ContentSHA256)
		}
		if len(pkg.ChangedFiles) > 0 {
			fmt.Fprintf(out, "  changed: %s\n", strings.Join(pkg.ChangedFiles, ", "))
		}
		if pkg.Error != "" {
			fmt.Fprintf(out, "  error: %s\n", pkg.Error)
		}
	}
}

func writeAdaptersText(out io.Writer, reports []AdapterReport) {
	for _, report := range reports {
		fmt.Fprintf(out, "%s: detected=%t version=%s config=%s\n", report.Provider, report.Detected, report.Version, report.ConfigDir)
		if report.Error != "" {
			fmt.Fprintf(out, "  error: %s\n", report.Error)
		}
		for _, route := range report.Routes {
			fmt.Fprintf(out, "  %s: available=%t installed=%t trusted=%t trust=%s stable_key=%t; %s\n", route.Name, route.Available, route.Installed, route.Trusted, route.Trust, route.StableKey, route.Detail)
			if route.Error != "" {
				fmt.Fprintf(out, "    error: %s\n", route.Error)
			}
		}
		fmt.Fprintf(out, "  limit: %d %s approximate=%t overflow=%s; %s\n", report.Limit.Value, report.Limit.Unit, report.Limit.Approximate, report.Limit.Overflow, report.Limit.Source)
		for _, slot := range report.Static {
			fmt.Fprintf(out, "  static %s %s: supported=%t; %s\n", slot.Name, slot.Mode, slot.Supported, slot.Reason)
		}
		for _, capability := range report.Capabilities {
			fmt.Fprintf(out, "  %s %s: available=%t; %s\n", capability.Point, capability.Name, capability.Available, capability.Reason)
		}
		for _, field := range report.Fields {
			fmt.Fprintf(out, "  %s %s: available=%t approximate=%t source=%s; %s\n", field.Point, field.Name, field.Available, field.Approximate, field.Source, field.Reason)
		}
	}
}
