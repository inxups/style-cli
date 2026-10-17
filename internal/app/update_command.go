package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const updateUsage = "style update <name|--all> [--ref ref] [--allow-replace] [--yes] [--json]"

func runUpdate(ctx context.Context, service *Service, args []string, out, errOut io.Writer) int {
	if isHelp(args) {
		fmt.Fprintln(out, updateUsage)
		return ExitOK
	}
	request, jsonOutput, err := parseUpdateArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style update: %v\n%s\n", err, updateUsage)
		return ExitUsage
	}
	previewWritten := false
	if !jsonOutput {
		yes := request.Yes
		request.Yes = false
		request.Confirm = func(preview UpdateResult) (bool, error) {
			writeUpdatePreview(out, preview)
			previewWritten = true
			return yes, nil
		}
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
			fmt.Fprintln(errOut, "approval required; review the update preview and rerun with --yes")
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
	request := UpdateRequest{}
	jsonOutput := false
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		flag, _, _ := strings.Cut(arg, "=")
		if strings.HasPrefix(arg, "--") {
			if seen[flag] {
				return request, false, fmt.Errorf("%s may only be specified once", flag)
			}
			seen[flag] = true
		}
		switch {
		case arg == "--all":
			request.All = true
		case arg == "--yes":
			request.Yes = true
		case arg == "--allow-replace":
			request.AllowReplace = true
		case arg == "--json":
			jsonOutput = true
		case arg == "--ref" || strings.HasPrefix(arg, "--ref="):
			path := ""
			if _, err := parseSourceOption(args, &i, &request.Ref, &path); err != nil {
				return request, false, err
			}
		case !strings.HasPrefix(arg, "-") && request.Name == "":
			request.Name = arg
		default:
			return request, false, fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if (request.Name == "") == !request.All {
		return request, false, fmt.Errorf("specify exactly one package name or --all")
	}
	if request.All && request.Ref != "" {
		return request, false, fmt.Errorf("--ref requires a single Git package")
	}
	return request, jsonOutput, nil
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
