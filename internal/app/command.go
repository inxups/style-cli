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
	if isHelp(args) {
		fmt.Fprintln(out, "style validate [source] [--ref ref] [--path repo-path] [--json]")
		return ExitOK
	}
	request, jsonOutput, err := parseValidateArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style validate: %v\n", err)
		return ExitUsage
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
		var report any = result.Diagnostics
		if source.Remote(request.Source) {
			report = struct {
				Source      *SourceReport `json:"source,omitempty"`
				Diagnostics []Diagnostic  `json:"diagnostics"`
			}{result.Source, result.Diagnostics}
		}
		if err := encodeJSON(out, report); err != nil {
			fmt.Fprintf(errOut, "style validate: encode report: %v\n", err)
			return ExitInvalid
		}
	} else {
		writeSource(out, result.Source)
		writeDiagnostics(out, result.Diagnostics)
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
	if isHelp(args) {
		fmt.Fprintln(out, "style plan <source> [--ref ref] [--path repo-path] [--harness auto|codex,claude-code] [--assume-requests N] [--assume-triggers N] "+countFlags+" [--json]")
		return ExitOK
	}
	request, jsonOutput, err := parsePlanArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style plan: %v\n", err)
		return ExitUsage
	}
	if request.Source == "" {
		fmt.Fprintln(errOut, "style plan: source is required")
		return ExitUsage
	}
	if !source.Remote(request.Source) && !filepath.IsAbs(request.Source) {
		request.Source = filepath.Join(workDir, request.Source)
	}
	result, err := service.Plan(ctx, request)
	if err != nil {
		fmt.Fprintf(errOut, "style plan: %v\n", err)
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			return ExitUsage
		}
		return ExitInvalid
	}
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style plan: encode report: %v\n", err)
			return ExitInvalid
		}
	} else {
		writeDiagnostics(out, result.Diagnostics)
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
	var request ValidateRequest
	jsonOutput := false
	for index := 0; index < len(args); index++ {
		if handled, err := parseSourceOption(args, &index, &request.Ref, &request.Path); handled {
			if err != nil {
				return ValidateRequest{}, false, err
			}
			continue
		}
		arg := args[index]
		switch arg {
		case "--json":
			jsonOutput = true
		case "--help", "-h":
			return ValidateRequest{}, false, errors.New("usage: style validate [source] [--ref ref] [--path repo-path] [--json]")
		default:
			if strings.HasPrefix(arg, "-") {
				return ValidateRequest{}, false, fmt.Errorf("unknown option %q", arg)
			}
			if request.Source != "" {
				return ValidateRequest{}, false, errors.New("only one source path is allowed")
			}
			request.Source = arg
		}
	}
	return request, jsonOutput, nil
}

func parsePlanArgs(args []string) (PlanRequest, bool, error) {
	request := PlanRequest{}
	jsonOutput := false
	for index := 0; index < len(args); index++ {
		if handled, err := parseCountOption(args, &index, &request.Count); handled {
			if err != nil {
				return PlanRequest{}, false, err
			}
			continue
		}
		if handled, err := parseSourceOption(args, &index, &request.Ref, &request.Path); handled {
			if err != nil {
				return PlanRequest{}, false, err
			}
			continue
		}
		arg := args[index]
		switch {
		case arg == "--json":
			jsonOutput = true
		case arg == "--harness":
			if index+1 >= len(args) {
				return PlanRequest{}, false, errors.New("--harness requires a value")
			}
			index++
			request.Harnesses = append(request.Harnesses, args[index])
		case strings.HasPrefix(arg, "--harness="):
			request.Harnesses = append(request.Harnesses, strings.TrimPrefix(arg, "--harness="))
		case arg == "--assume-requests", arg == "--assume-triggers":
			if index+1 >= len(args) {
				return PlanRequest{}, false, fmt.Errorf("%s requires a value", arg)
			}
			value, err := strconv.Atoi(args[index+1])
			if err != nil || value <= 0 {
				return PlanRequest{}, false, fmt.Errorf("%s requires a positive integer", arg)
			}
			if arg == "--assume-requests" {
				request.AssumeRequests = value
			} else {
				request.AssumeTriggers = value
			}
			index++
		case strings.HasPrefix(arg, "--assume-requests="):
			value, err := positiveInt(strings.TrimPrefix(arg, "--assume-requests="))
			if err != nil {
				return PlanRequest{}, false, fmt.Errorf("--assume-requests: %w", err)
			}
			request.AssumeRequests = value
		case strings.HasPrefix(arg, "--assume-triggers="):
			value, err := positiveInt(strings.TrimPrefix(arg, "--assume-triggers="))
			if err != nil {
				return PlanRequest{}, false, fmt.Errorf("--assume-triggers: %w", err)
			}
			request.AssumeTriggers = value
		case arg == "--help", arg == "-h":
			return PlanRequest{}, false, errors.New("usage: style plan <source> [--harness auto|h1,h2]")
		default:
			if strings.HasPrefix(arg, "-") {
				return PlanRequest{}, false, fmt.Errorf("unknown option %q", arg)
			}
			if request.Source != "" {
				return PlanRequest{}, false, errors.New("only one source path is allowed")
			}
			request.Source = arg
		}
	}
	return request, jsonOutput, nil
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

func isHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
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
