package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

func parseFrozenArgs(args []string) (FrozenRequest, bool, error) {
	parsed, err := frozenCommand.parse(args)
	if err != nil {
		return FrozenRequest{}, false, err
	}
	return FrozenRequest{Yes: parsed.Bool("--yes"), AllowReplace: parsed.Bool("--allow-replace")}, parsed.Bool("--json"), nil
}

func runFrozen(ctx context.Context, s *Service, args []string, out, errOut io.Writer) int {
	request, jsonOutput, err := parseFrozenArgs(args)
	if err != nil {
		return frozenCommand.usageError(errOut, err)
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
