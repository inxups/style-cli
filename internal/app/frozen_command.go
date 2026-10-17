package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const frozenUsage = "style install --frozen [--allow-replace] [--yes] [--json]"

func parseFrozenArgs(args []string) (FrozenRequest, bool, error) {
	request := FrozenRequest{}
	jsonOutput := false
	seen := map[string]bool{}
	for _, arg := range args {
		if seen[arg] {
			return request, false, fmt.Errorf("%s may only be specified once", arg)
		}
		seen[arg] = true
		switch arg {
		case "--frozen":
		case "--yes":
			request.Yes = true
		case "--allow-replace":
			request.AllowReplace = true
		case "--json":
			jsonOutput = true
		default:
			return request, false, fmt.Errorf("unexpected frozen install argument %q", arg)
		}
	}
	if !seen["--frozen"] {
		return request, false, fmt.Errorf("--frozen is required")
	}
	return request, jsonOutput, nil
}

func runFrozen(ctx context.Context, s *Service, args []string, out, errOut io.Writer) int {
	if len(args) == 2 && (args[0] == "--help" || args[1] == "--help" || args[0] == "-h" || args[1] == "-h") {
		fmt.Fprintln(out, frozenUsage)
		return ExitOK
	}
	request, jsonOutput, err := parseFrozenArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style install: %v\n%s\n", err, frozenUsage)
		return ExitUsage
	}
	written := false
	if !jsonOutput {
		yes := request.Yes
		request.Yes = false
		request.Confirm = func(preview FrozenResult) (bool, error) {
			writeFrozenPreview(out, preview)
			written = true
			return yes, nil
		}
	}
	result, runErr := s.InstallFrozen(ctx, request)
	if jsonOutput {
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style install: %v\n", err)
			return ExitInvalid
		}
	} else {
		if !written && runErr == nil {
			writeFrozenPreview(out, result)
		}
		if runErr != nil {
			writeDiagnostics(errOut, result.Diagnostics)
		} else if result.AwaitingConfirmation {
			fmt.Fprintln(errOut, "approval required; review the frozen recovery and rerun with --yes")
		} else {
			fmt.Fprintf(out, "frozen install %s (changed=%t)\n", result.Status, result.Changed)
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

func writeFrozenPreview(out io.Writer, result FrozenResult) {
	fmt.Fprintf(out, "Lock: %s; locked_source=%t\n", result.LockSHA256, result.LockedSource)
	for _, content := range result.Contents {
		fmt.Fprintf(out, "  %s: snapshot=%s storage_repair=%t\n", content.Package, content.Snapshot, content.StorageRepair)
	}
	writeSourceTransactionPreview(out, result.UpdateResult, "Frozen install")
}
