package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

func parseFrozenArgs(args []string) (FrozenRequest, bool, error) {
	parsed, err := restoreCommand.parse(args)
	if err != nil {
		return FrozenRequest{}, false, err
	}
	return FrozenRequest{AllowReplace: parsed.Bool("--allow-replace")}, parsed.Bool("--json"), nil
}

func runRestore(ctx context.Context, s *Service, args []string, in io.Reader, out, errOut io.Writer) int {
	request, jsonOutput, err := parseFrozenArgs(args)
	if err != nil {
		return restoreCommand.usageError(errOut, err)
	}
	written := false
	request.Confirm = func(preview FrozenResult) (bool, error) {
		previewOut := out
		if jsonOutput {
			previewOut = errOut
		}
		writeFrozenPreview(previewOut, preview)
		written = !jsonOutput
		return confirmInput(in, errOut, "Apply this restore?"), nil
	}
	result, runErr := s.InstallFrozen(ctx, request)
	if jsonOutput {
		if runErr == nil && result.AwaitingConfirmation {
			result.Diagnostics = append(result.Diagnostics, declinedDiagnostic("restore"))
		}
		if err := encodeJSON(out, result); err != nil {
			fmt.Fprintf(errOut, "style restore: encode report: %v\n", err)
			return ExitInvalid
		}
	} else {
		if !written && runErr == nil {
			writeFrozenPreview(out, result)
		}
		if runErr != nil {
			writeDiagnostics(errOut, result.Diagnostics)
		} else if result.AwaitingConfirmation {
			fmt.Fprintln(errOut, "restore canceled; nothing was changed")
		} else {
			fmt.Fprintf(out, "restore %s (changed=%t)\n", result.Status, result.Changed)
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
	writeSourceTransactionPreview(out, result.UpdateResult, "Restore")
}
