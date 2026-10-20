package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// runSync applies source changes. Changes that need approval are previewed
// and confirmed one package at a time: "sync NAME" approves that package, and
// a bare "sync" walks every package that needs approval. Each approval is its
// own transaction, so what is applied is exactly what was previewed.
func runSync(ctx context.Context, service *Service, args []string, in io.Reader, out, errOut io.Writer) int {
	parsed, err := syncCommand.parse(args)
	if err != nil {
		return syncCommand.usageError(errOut, err)
	}
	jsonOutput, named := parsed.Bool("--json"), parsed.Arg(0)
	fail := func(err error) int {
		var validation *ValidationError
		if errors.As(err, &validation) {
			if jsonOutput {
				_ = encodeJSON(out, validation.Diagnostics)
			} else {
				writeCommandDiagnostics(errOut, "sync", validation.Diagnostics)
			}
		} else {
			commandFailure(out, errOut, jsonOutput, "sync", err)
		}
		return commandExit(err)
	}
	previewOut := out
	if jsonOutput {
		previewOut = errOut
	}
	previewed := false
	approve := func(name string) (SyncResult, error) {
		previewed = false
		return service.Trust(ctx, TrustRequest{Name: name, AllowReplace: parsed.Bool("--allow-replace"), Confirm: func(preview SyncResult) (bool, error) {
			writeSyncChanges(previewOut, preview, true)
			previewed = true
			return confirmInput(in, errOut, "Approve "+name+"?"), nil
		}})
	}
	var result SyncResult
	var approved []string
	var approvedChanges []PackageChange
	changed := false
	record := func(name string, result SyncResult) {
		approved = append(approved, name)
		changed = changed || result.Changed
		for _, change := range result.Changes {
			if change.Package == name {
				approvedChanges = append(approvedChanges, change)
			}
		}
	}
	if named != "" {
		if result, err = approve(named); err != nil {
			return fail(err)
		}
		if previewed && !result.AwaitingConfirmation {
			record(named, result)
		}
	} else {
		for {
			if result, err = service.Sync(ctx, SyncRequest{}); err != nil {
				return fail(err)
			}
			if len(result.NeedsTrust) == 0 {
				break
			}
			name := result.NeedsTrust[0]
			if slices.Contains(approved, name) {
				return fail(&ConflictError{Message: fmt.Sprintf("package %s still needs approval after it was approved", name)})
			}
			if result, err = approve(name); err != nil {
				return fail(err)
			}
			if result.AwaitingConfirmation {
				break
			}
			record(name, result)
		}
	}
	result.Approved = approved
	result.Changed = result.Changed || changed
	if result.AwaitingConfirmation {
		declined := declinedDiagnostic("sync")
		if len(approved) > 0 {
			declined.Message = fmt.Sprintf("sync canceled; already approved and applied: %s", strings.Join(approved, ", "))
		}
		if jsonOutput {
			result.Diagnostics = append(result.Diagnostics, declined)
			_ = encodeJSON(out, result)
		} else {
			writeCommandDiagnostics(errOut, "sync", []Diagnostic{declined})
		}
		return ExitInvalid
	}
	if named == "" && len(approvedChanges) > 0 {
		result.Changes = append(approvedChanges, result.Changes...)
	}
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style sync: encode report: %v\n", err)
			return ExitInvalid
		}
		return ExitOK
	}
	if !previewed || named == "" {
		writeSyncChanges(out, result, named != "" && !previewed)
	}
	if len(approved) > 0 {
		fmt.Fprintf(out, "approved: %s\n", strings.Join(approved, ", "))
	}
	fmt.Fprintf(out, "sync complete (changed=%t, expired sessions=%d)\n", result.Changed, result.ExpiredSessions)
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
			if len(change.SourceDiff) > 0 {
				fmt.Fprintln(out, "Source diff:")
			}
			for _, file := range change.SourceDiff {
				fmt.Fprint(out, file.Diff)
			}
			if len(change.RenderedDiff) > 0 {
				fmt.Fprintln(out, "Rendered diff (static values substituted; event variables remain runtime fragments):")
			}
			for _, file := range change.RenderedDiff {
				fmt.Fprint(out, file.Diff)
			}
		}
	}
	writeStaticMigrations(out, result.StaticMigrations)
	for _, name := range result.NeedsTrust {
		fmt.Fprintf(out, "approval required: style sync %s\n", name)
	}
	for _, report := range result.Reports {
		writePlanText(out, PlanResult{Reports: []PlanReport{report}})
	}
	if full {
		writeNativeHooks(out, result.Native)
	}
	writeDiagnostics(out, result.Diagnostics)
}
