package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/source"
)

func runValidate(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	request, jsonOutput, err := parseValidateArgs(args)
	if err != nil {
		return validateCommand.usageError(errOut, err)
	}
	if request.Source == "" {
		request.Source = workDir
	} else if !source.Remote(request.Source) && !filepath.IsAbs(request.Source) {
		request.Source = filepath.Join(workDir, request.Source)
	}
	result, err := service.Validate(ctx, request)
	if err != nil {
		fmt.Fprintf(errOut, "style validate: %v\n", err)
		return ExitInvalid
	}
	if jsonOutput {
		report := struct {
			Source      *SourceReport `json:"source,omitempty"`
			Diagnostics []Diagnostic  `json:"diagnostics"`
		}{result.Source, result.Diagnostics}
		if err := encodeJSON(out, report); err != nil {
			fmt.Fprintf(errOut, "style validate: encode report: %v\n", err)
			return ExitInvalid
		}
	} else {
		writeSource(out, result.Source)
		writeDiagnostics(errOut, result.Diagnostics)
		if len(result.Diagnostics) == 0 {
			fmt.Fprintln(out, "valid")
		}
	}
	if !result.Valid() {
		return ExitInvalid
	}
	return ExitOK
}

func runPlan(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	request, jsonOutput, err := parsePlanArgs(args)
	if err == nil {
		_, err = service.parseHarnesses(request.Harnesses)
	}
	if err != nil {
		return planCommand.usageError(errOut, err)
	}
	if request.Source == "" {
		request.Source = workDir
	} else if !source.Remote(request.Source) && !filepath.IsAbs(request.Source) {
		request.Source = filepath.Join(workDir, request.Source)
	}
	result, err := service.Plan(ctx, request)
	if err != nil {
		commandFailure(out, errOut, jsonOutput, "plan", err)
		return commandExit(err)
	}
	if jsonOutput {
		var report any = result
		if result.Package == "" && len(result.Reports) == 0 {
			// Like the text output, a package that did not load has no plan.
			report = struct {
				Diagnostics []Diagnostic `json:"diagnostics"`
			}{result.Diagnostics}
		}
		if err := encodeJSON(out, report); err != nil {
			fmt.Fprintf(errOut, "style plan: encode report: %v\n", err)
			return ExitInvalid
		}
	} else {
		writeDiagnostics(errOut, result.Diagnostics)
		if result.Package != "" || len(result.Reports) > 0 {
			writePlanText(out, result)
			for _, report := range result.Reports {
				for _, count := range report.Counts {
					fmt.Fprintf(out, "count %s %s %s: ", report.Provider, count.Rule, count.Point)
					writeCountText(out, count.Count)
				}
				if report.CountScope != "" {
					fmt.Fprintln(out, report.CountScope)
				}
			}
		}
	}
	if hasErrors(result.Diagnostics) {
		return ExitInvalid
	}
	for _, report := range result.Reports {
		if report.Plan.Blocked {
			return ExitBlocked
		}
		for _, diagnostic := range report.Tokens.Diagnostics {
			if diagnostic.Severity == spec.SeverityError {
				return ExitBlocked
			}
		}
	}
	return ExitOK
}

func parseValidateArgs(args []string) (ValidateRequest, bool, error) {
	parsed, err := validateCommand.parse(args)
	if err != nil {
		return ValidateRequest{}, false, err
	}
	if err := sourceArgument(parsed.Arg(0)); err != nil {
		return ValidateRequest{}, false, err
	}
	return ValidateRequest{Source: parsed.Arg(0)}, parsed.Bool("--json"), nil
}

func parsePlanArgs(args []string) (PlanRequest, bool, error) {
	parsed, err := planCommand.parse(args)
	if err != nil {
		return PlanRequest{}, false, err
	}
	request := PlanRequest{Source: parsed.Arg(0), Harnesses: parsed.List("--harness")}
	if err := sourceArgument(request.Source); err != nil {
		return PlanRequest{}, false, err
	}
	if request.Count, err = countOptions(parsed); err != nil {
		return PlanRequest{}, false, err
	}
	for _, option := range []struct {
		name  string
		value *int
	}{{"--assume-requests", &request.AssumeRequests}, {"--assume-triggers", &request.AssumeTriggers}} {
		if parsed.Has(option.name) {
			if *option.value, err = positiveInt(parsed.String(option.name)); err != nil {
				return PlanRequest{}, false, fmt.Errorf("%s %w", option.name, err)
			}
		}
	}
	return request, parsed.Bool("--json"), nil
}

func positiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("requires a positive integer")
	}
	return parsed, nil
}

func writeDiagnostics(out io.Writer, diagnostics []Diagnostic) {
	for _, diagnostic := range diagnostics {
		where := diagnostic.Path
		if where == "" {
			where = diagnostic.Rule
		}
		if diagnostic.Provider != "" {
			if where != "" {
				where += " (" + diagnostic.Provider + ")"
			} else {
				where = diagnostic.Provider
			}
		}
		if where == "" {
			fmt.Fprintf(out, "%s: %s\n", diagnostic.Code, diagnostic.Message)
		} else {
			fmt.Fprintf(out, "%s %s: %s\n", diagnostic.Code, where, diagnostic.Message)
		}
	}
}

func writePlanText(out io.Writer, result PlanResult) {
	writeSource(out, result.Source)
	fmt.Fprintf(out, "%s %s\n", result.Package, result.Version)
	for _, report := range result.Reports {
		fmt.Fprintf(out, "\nHarness: %s\n", report.Provider)
		fmt.Fprintf(out, "Preferred route: %s (availability checked during install)\n", report.Route)
		fmt.Fprint(out, report.Plan.String())
		fmt.Fprintln(out, "Token (estimated; calibration pending):")
		fmt.Fprintf(out, "  native input limit: %d %s", report.NativeLimit.Value, report.NativeLimit.Unit)
		if report.NativeLimit.Approximate {
			fmt.Fprint(out, " (approximate)")
		}
		if report.NativeLimit.Overflow != "" {
			fmt.Fprintf(out, "; overflow=%s", report.NativeLimit.Overflow)
		}
		if report.NativeLimit.Source != "" {
			fmt.Fprintf(out, "; source=%s", report.NativeLimit.Source)
		}
		fmt.Fprintln(out)
		fmt.Fprintf(out, "  budget: per_rule %s; per_event %s; always_on %s tokens\n", formatNumber(report.Budget.PerRule), formatNumber(report.Budget.PerEvent), formatNumber(report.Budget.AlwaysOn))
		fmt.Fprintf(out, "  always-on: %d tokens / %d chars\n", report.Tokens.AlwaysOn.Tokens, report.Tokens.AlwaysOn.Chars)
		fmt.Fprintln(out, "  per-rule:")
		for _, rule := range report.Tokens.Rules {
			fmt.Fprintf(out, "    %s: %d tokens / %d chars (%s)\n", rule.RuleID, rule.Estimate.Tokens, rule.Estimate.Chars, rule.Status)
			if rule.DeduplicatedInto != "" {
				fmt.Fprintf(out, "      deduplicated into %s; no additional cost\n", rule.DeduplicatedInto)
			}
		}
		for _, event := range report.Tokens.Events {
			fmt.Fprintf(out, "  %s: %d tokens / %d chars\n", event.Label, event.Estimate.Tokens, event.Estimate.Chars)
		}
		if len(report.Tokens.Unestimated) > 0 {
			hooks := make([]string, len(report.Tokens.Unestimated))
			for i, hook := range report.Tokens.Unestimated {
				hooks[i] = fmt.Sprintf("%s (%s)", hook.RuleID, hook.Event)
			}
			fmt.Fprintf(out, "  native hook output not estimated: %s\n", strings.Join(hooks, ", "))
		}
		for _, diagnostic := range report.Tokens.Diagnostics {
			fmt.Fprintf(out, "  %s: %s\n", diagnostic.Code, diagnostic.Message)
		}
		fmt.Fprintf(out, "  session cost (requests=%d triggers=%d): %.1f always-on + %.1f triggered = %.1f units\n", report.SessionCost.Requests, report.SessionCost.Triggers, report.SessionCost.AlwaysOn, report.SessionCost.Triggered, report.SessionCost.Total)
		fmt.Fprintln(out, "  scenario only: 1.25x cache write + 0.1x later hits; assumes an eligible unchanged prefix within cache lifetime; not billing")
	}
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func encodeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
