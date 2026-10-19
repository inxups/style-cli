package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

func runSync(ctx context.Context, service *Service, command string, args []string, in io.Reader, out, errOut io.Writer) int {
	spec, _ := lookupCommand(command)
	parsed, err := spec.parse(args)
	if err == nil && command == "trust" && parsed.Arg(0) == "" {
		err = errors.New("package name is required")
	}
	if err != nil {
		return spec.usageError(errOut, err)
	}
	request := TrustRequest{Name: parsed.Arg(0), AllowReplace: parsed.Bool("--allow-replace")}
	jsonOutput := parsed.Bool("--json")
	var result SyncResult
	previewWritten := false
	if command == "trust" {
		request.Confirm = func(preview SyncResult) (bool, error) {
			previewOut := out
			if jsonOutput {
				previewOut = errOut
			}
			writeSyncChanges(previewOut, preview, true)
			previewWritten = !jsonOutput
			return confirmInput(in, errOut, "Approve this snapshot?"), nil
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
			fmt.Fprintln(errOut, "trust canceled; nothing was approved")
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
