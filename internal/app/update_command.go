package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/inxups/style-cli/internal/source"
)

func runUpdate(ctx context.Context, service *Service, args []string, in io.Reader, out, errOut io.Writer) int {
	request, jsonOutput, err := parseUpdateArgs(args)
	if err != nil {
		return updateCommand.usageError(errOut, err)
	}
	previewWritten := false
	request.Confirm = func(preview UpdateResult) (bool, error) {
		previewOut := out
		if jsonOutput {
			previewOut = errOut
		}
		writeUpdatePreview(previewOut, preview)
		previewWritten = !jsonOutput
		return confirmInput(in, errOut, "Apply this update?"), nil
	}
	result, runErr := service.Update(ctx, request)
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style update: %v\n", err)
			return ExitInvalid
		}
	} else {
		if !previewWritten && runErr == nil {
			writeUpdatePreview(out, result)
		}
		if runErr != nil {
			// Only the sanitized structured diagnostics go to routine errors.
			writeDiagnostics(errOut, result.Diagnostics)
		} else if result.AwaitingConfirmation {
			fmt.Fprintln(errOut, "update canceled; nothing was changed")
		} else {
			fmt.Fprintf(out, "update %s (changed=%t)\n", result.Status, result.Changed)
		}
	}
	var blocked *BlockedError
	if errors.As(runErr, &blocked) {
		return ExitBlocked
	}
	if runErr != nil || result.AwaitingConfirmation {
		return ExitInvalid
	}
	return ExitOK
}

func parseUpdateArgs(args []string) (UpdateRequest, bool, error) {
	parsed, err := updateCommand.parse(args)
	if err != nil {
		return UpdateRequest{}, false, err
	}
	request := UpdateRequest{Name: parsed.Arg(0), All: parsed.Bool("--all"), Ref: parsed.String("--ref"), AllowReplace: parsed.Bool("--allow-replace")}
	if (request.Name == "") == !request.All {
		return request, false, fmt.Errorf("specify exactly one package name or --all")
	}
	if request.All && request.Ref != "" {
		return request, false, fmt.Errorf("--ref requires a single Git package")
	}
	if request.Ref != "" && !source.ValidRef(request.Ref) {
		return request, false, errors.New("source-invalid: --ref requires a full branch/tag ref or commit OID")
	}
	return request, parsed.Bool("--json"), nil
}

func writeUpdatePreview(out io.Writer, result UpdateResult) {
	writeSourceTransactionPreview(out, result, "Update")
}

func writeSourceTransactionPreview(out io.Writer, result UpdateResult, heading string) {
	fmt.Fprintf(out, "%s: %s; selected=%q\n", heading, result.Status, result.Selected)
	for _, change := range result.Changes {
		fmt.Fprintf(out, "%s (seq=%d):\n", change.Package, change.Sequence)
		for _, side := range []struct {
			label    string
			revision *UpdateRevision
		}{{"before", &change.Before}, {"after", change.After}} {
			if side.revision == nil {
				continue
			}
			r := side.revision
			fmt.Fprintf(out, "  %s: version=%q source=%q kind=%s link=%t link_trusted=%t replace=%t content=%s\n",
				side.label, r.Version, r.Source.Path, r.Source.Kind, r.Source.Link, r.LinkTrusted, r.Replace, r.ContentSHA256)
			if r.Source.Git != nil {
				git := r.Source.Git
				fmt.Fprintf(out, "    Git: %q ref=%q commit=%s object=%s:%s\n", git.URL, git.Ref, git.Commit, git.ObjectFormat, git.Object)
			}
		}
		if change.Acquisition != nil {
			fmt.Fprintf(out, "  Acquisition: Git %s; cache_hit=%t (source cache is separate from installation writes)\n", change.Acquisition.GitVersion, change.Acquisition.CacheHit)
		}
		fmt.Fprintf(out, "  settings added=%q removed=%q; approval_changed=%t configuration_repair=%t\n",
			change.SettingsAdded, change.SettingsRemoved, change.ApprovalChanged, change.ConfigurationRepair)
		if !change.BaselineAvailable {
			fmt.Fprintln(out, "  previous authenticated rendered baseline unavailable; review the complete candidate effects")
		}
		if change.NativeChanged {
			fmt.Fprintln(out, "  native declaration/order/configuration change; renewed approval required")
		}
		fmt.Fprintln(out, "Source diff:")
		for _, file := range change.SourceDiff {
			fmt.Fprint(out, file.Diff)
		}
		fmt.Fprintln(out, "Rendered diff (static values substituted; event variables remain runtime fragments):")
		for _, file := range change.RenderedDiff {
			fmt.Fprint(out, file.Diff)
		}
		if len(change.NativeBefore) > 0 {
			fmt.Fprintln(out, "Previous approved native order:")
			writeNativeHooks(out, change.NativeBefore)
		}
	}
	for _, report := range result.Reports {
		fmt.Fprintf(out, "Plan: %s\n", report.Package)
		writePlanText(out, PlanResult{Reports: []PlanReport{report.PlanReport}})
	}
	fmt.Fprintf(out, "Locked routes: %q\n", result.Routes)
	writeNativeHooks(out, result.Native)
	writeDiagnostics(out, result.Diagnostics)
}
