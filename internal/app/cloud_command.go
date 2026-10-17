package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

const exportUsage = "style export --target claude-cloud --project <dir> [--yes] [--json]"

func runExport(ctx context.Context, s *Service, args []string, out, errOut io.Writer) int {
	if isHelp(args) {
		fmt.Fprintln(out, exportUsage)
		return ExitOK
	}
	var request ExportRequest
	jsonMode := false
	seen := map[string]bool{}
	fail := func(err error) int {
		fmt.Fprintln(errOut, "style export:", err)
		if jsonMode {
			_ = encodeJSON(out, map[string]any{"status": "error", "error": err.Error()})
		}
		return ExitInvalid
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes":
			request.Yes = true
		case arg == "--json":
			jsonMode = true
		case arg == "--target" || strings.HasPrefix(arg, "--target="):
			if seen["target"] {
				return fail(&UsageError{Message: "duplicate --target"})
			}
			seen["target"] = true
			if arg == "--target" {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
					return fail(&UsageError{Message: "--target requires a value"})
				}
				i++
				request.Target = args[i]
			} else {
				request.Target = strings.TrimPrefix(arg, "--target=")
			}
		default:
			return fail(&UsageError{Message: "unknown export argument " + arg})
		}
	}
	result, err := s.Export(ctx, request)
	if err != nil {
		return fail(err)
	}
	if jsonMode {
		if encodeJSON(out, result) != nil {
			return ExitInvalid
		}
	} else {
		fmt.Fprintf(out, "target=%s changed=%t awaiting_confirmation=%t\n", result.Target, result.Changed, result.AwaitingConfirmation)
		for _, warning := range result.Warnings {
			fmt.Fprintln(out, warning)
		}
		names := make([]string, 0, len(result.Content))
		for name := range result.Content {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(out, "\n--- %s ---\n%s\n", name, result.Content[name])
		}
		writeNativeHooks(out, result.Native)
	}
	if result.AwaitingConfirmation {
		return ExitInvalid
	}
	return ExitOK
}
