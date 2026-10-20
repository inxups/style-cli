package app

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func runInspect(ctx context.Context, service *Service, workDir, command string, args []string, in io.Reader, out, errOut io.Writer) int {
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
		request := DoctorRequest{WorkDir: workDir, ResetState: reset}
		if reset {
			request.Confirm = func(root string) (bool, error) {
				return confirmInput(in, errOut, "Remove all runtime state in "+root+", including sessions and rule statistics?"), nil
			}
		}
		result, err = service.Doctor(ctx, request)
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
		// Problems lead the report so they are not buried under the matrix.
		writeDiagnostics(out, result.Diagnostics)
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
			fmt.Fprintf(out, "effort %s: %s%s\n", effort.Provider, textFields("source", effort.Source, "raw", quoteNonEmpty(effort.Raw), "tier", quoteNonEmpty(effort.Tier)), textReason(effort.Detail))
		}
		if len(result.TopRules) == 0 {
			fmt.Fprintln(out, "Top rules (last 7 days): none")
		} else {
			fmt.Fprintln(out, "Top rules (last 7 days; estimated tokens; calibration pending):")
		}
		for _, cost := range result.TopRules {
			fmt.Fprintf(out, "  %s %s: %d tokens, %d emits, %d body hashes\n", cost.Provider, cost.Rule, cost.EstimatedTokens, cost.Emits, cost.DistinctBodyHashes)
		}
		fmt.Fprintf(out, "state: expired=%d reset=%t; legacy stats without estimate=%d\n", result.ExpiredSessions, result.StateReset, result.LegacyStats)
		if !result.Healthy {
			// The report stays on stdout; stderr says why the exit code is 1.
			summary := Diagnostic{Code: "unhealthy", Severity: "error", Message: "the installation is not healthy"}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == "confirmation-declined" {
					summary = diagnostic
					break
				}
				if diagnostic.Severity == "error" && summary.Code == "unhealthy" {
					summary = diagnostic
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
		fmt.Fprintf(out, "%s: %s\n", report.Provider, textFields("detected", strconv.FormatBool(report.Detected), "version", report.Version, "config", report.ConfigDir))
		if report.Error != "" {
			fmt.Fprintf(out, "  error: %s\n", report.Error)
		}
		for _, route := range report.Routes {
			fmt.Fprintf(out, "  %s: available=%t installed=%t trusted=%t trust=%s stable_key=%t%s\n", route.Name, route.Available, route.Installed, route.Trusted, route.Trust, route.StableKey, textReason(route.Detail))
			if route.Error != "" {
				fmt.Fprintf(out, "    error: %s\n", route.Error)
			}
		}
		fmt.Fprintf(out, "  limit: %d %s approximate=%t overflow=%s%s\n", report.Limit.Value, report.Limit.Unit, report.Limit.Approximate, report.Limit.Overflow, textReason(report.Limit.Source))
		for _, slot := range report.Static {
			fmt.Fprintf(out, "  static %s %s: supported=%t%s\n", slot.Name, slot.Mode, slot.Supported, textReason(slot.Reason))
		}
		if !report.Detected {
			// The per-point matrix of a harness that is not present is noise
			// here; --json keeps it.
			fmt.Fprintf(out, "  %d outputs and %d event fields not listed; harness not detected (see --json)\n", len(report.Capabilities), len(report.Fields))
			continue
		}
		for _, capability := range report.Capabilities {
			fmt.Fprintf(out, "  %s %s: available=%t%s\n", capability.Point, capability.Name, capability.Available, textReason(capability.Reason))
		}
		for _, field := range report.Fields {
			fmt.Fprintf(out, "  %s %s: %s%s\n", field.Point, field.Name, textFields("available", strconv.FormatBool(field.Available), "approximate", strconv.FormatBool(field.Approximate), "source", field.Source), textReason(field.Reason))
		}
	}
}

// textReason appends a "; reason" clause only when there is a reason.
func textReason(reason string) string {
	if reason == "" {
		return ""
	}
	return "; " + reason
}

// quoteNonEmpty quotes a value for a text field, leaving an empty one empty
// so textFields drops it.
func quoteNonEmpty(value string) string {
	if value == "" {
		return ""
	}
	return strconv.Quote(value)
}
