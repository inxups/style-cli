package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

func runSync(ctx context.Context, service *Service, command string, args []string, out, errOut io.Writer) int {
	usage := "style sync [--json]"
	if command == "trust" {
		usage = "style trust <name> [--allow-replace] [--yes] [--json]"
	}
	if isHelp(args) {
		fmt.Fprintln(out, usage)
		return ExitOK
	}
	request := TrustRequest{}
	jsonOutput := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOutput = true
		case arg == "--yes" && command == "trust":
			request.Yes = true
		case arg == "--allow-replace" && command == "trust":
			request.AllowReplace = true
		case command == "trust" && !strings.HasPrefix(arg, "-") && request.Name == "":
			request.Name = arg
		default:
			fmt.Fprintf(errOut, "style %s: unexpected argument %q\n", command, arg)
			return ExitUsage
		}
	}
	if command == "trust" && request.Name == "" {
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	var result SyncResult
	var err error
	previewWritten := false
	if command == "trust" {
		if !jsonOutput {
			yes := request.Yes
			request.Yes = false
			request.Confirm = func(preview SyncResult) (bool, error) {
				writeSyncChanges(out, preview, true)
				previewWritten = true
				return yes, nil
			}
		}
		result, err = service.Trust(ctx, request)
	} else {
		result, err = service.Sync(ctx, SyncRequest{})
	}
	if err != nil {
		var validation *ValidationError
		var blocked *BlockedError
		if errors.As(err, &validation) {
			if jsonOutput {
				_ = encodeJSON(out, validation.Diagnostics)
			} else {
				writeDiagnostics(out, validation.Diagnostics)
			}
		} else {
			fmt.Fprintf(errOut, "style %s: %v\n", command, err)
		}
		if errors.As(err, &blocked) {
			return ExitBlocked
		}
		return ExitInvalid
	}
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style %s: %v\n", command, err)
			return ExitInvalid
		}
	} else {
		if !previewWritten {
			writeSyncChanges(out, result, command == "trust")
		}
		if result.AwaitingConfirmation {
			fmt.Fprintln(errOut, "approval required; review the diff and rerun trust with --yes")
		} else if command == "trust" || len(result.NeedsTrust) == 0 {
			fmt.Fprintf(out, "%s complete (changed=%t, expired sessions=%d)\n", command, result.Changed, result.ExpiredSessions)
		}
	}
	if command == "sync" && len(result.NeedsTrust) > 0 {
		return ExitSyncChanged
	}
	if result.AwaitingConfirmation {
		return ExitInvalid
	}
	return ExitOK
}

func writeSyncChanges(out io.Writer, result SyncResult, full bool) {
	for _, change := range result.Changes {
		fmt.Fprintf(out, "%s (link=%t): %d changed files\n", change.Package, change.Link, len(change.Files))
		for _, file := range change.Files {
			fmt.Fprintf(out, "  %s\n", file)
		}
		if change.NativeChanged {
			fmt.Fprintln(out, "  native declaration/order/configuration change; renewed approval required")
		}
		if full && change.NeedsTrust && change.Package != result.ApprovalTarget {
			fmt.Fprintln(out, "keeping the approved snapshot; current source is excluded from this approval")
			continue
		}
		if full {
			if len(change.NativeDiff) > 0 || change.NativeChanged {
				fmt.Fprintln(out, "Native entry diff:")
				for _, native := range change.NativeDiff {
					fmt.Fprintf(out, "  %s %s\n", native.Harness, native.Rule)
					for _, side := range []struct {
						label string
						hook  *NativeHookReport
					}{{"before", native.Before}, {"after", native.After}} {
						if side.hook == nil {
							fmt.Fprintf(out, "    %s: absent\n", side.label)
						} else {
							declared := "default"
							if side.hook.DeclaredTimeout > 0 {
								declared = fmt.Sprintf("%ds", side.hook.DeclaredTimeout)
							}
							fmt.Fprintf(out, "    %s: event=%s matcher=%q timeout=%ds (declared: %s) command=%s\n", side.label, side.hook.Event, side.hook.Matcher, side.hook.Timeout, declared, side.hook.Command)
						}
					}
				}
				if len(change.NativeDiff) == 0 {
					fmt.Fprintln(out, "  effective command fields unchanged; review declared timeout/order changes and configuration repair")
				}
			}
			if !change.BaselineAvailable {
				fmt.Fprintln(out, "previous approved bytes unavailable; showing the complete current source and compiled representation")
			}
			fmt.Fprintln(out, "Source diff:")
			for _, file := range change.SourceDiff {
				fmt.Fprint(out, file.Diff)
			}
			fmt.Fprintln(out, "Rendered diff (static values substituted; event variables remain runtime fragments):")
			for _, file := range change.RenderedDiff {
				fmt.Fprint(out, file.Diff)
			}
		}
	}
	writeStaticMigrations(out, result.StaticMigrations)
	for _, name := range result.NeedsTrust {
		fmt.Fprintf(out, "approval required: style trust %s\n", name)
	}
	for _, report := range result.Reports {
		writePlanText(out, PlanResult{Reports: []PlanReport{report}})
	}
	if full {
		writeNativeHooks(out, result.Native)
	}
	writeDiagnostics(out, result.Diagnostics)
}
