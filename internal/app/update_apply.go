package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/speakeasy-api/agenthooks/install"
)

type updateSwap interface {
	apply() error
	commit()
	rollback() error
}

// Internal ports permit deterministic faults at each transaction boundary.
// Production always uses packageSwap and agenthooks.ApplyFiles; no environment
// or command option enables these ports.
type updateDependencies struct {
	stage func(string, string, bool, map[string][]byte) (updateSwap, error)
	files func(context.Context, []install.PlannedFile) error
}

type updateRollbackError struct{ cause error }

func (e *updateRollbackError) Error() string { return e.cause.Error() }
func (e *updateRollbackError) Unwrap() error { return e.cause }

func applyUpdate(ctx context.Context, prepared preparedUpdate, deps updateDependencies) (result UpdateResult, runErr error) {
	result = prepared.result
	if err := checkContext(ctx); err != nil {
		return result, err
	}
	guard, err := acquireManagement(ctx, prepared.home)
	if err != nil {
		return result, err
	}
	defer func() { runErr = errors.Join(runErr, guard.close()) }()
	if err := prepared.inputs.verify(); err != nil {
		return result, err
	}
	if deps.stage == nil {
		deps.stage = func(home, name string, single bool, files map[string][]byte) (updateSwap, error) {
			return stagePackageFiles(home, name, single, files)
		}
	}
	if deps.files == nil {
		deps.files = install.ApplyFiles
	}
	var swaps []updateSwap
	committed := false
	defer func() {
		if committed {
			return
		}
		for i := len(swaps) - 1; i >= 0; i-- {
			if err := swaps[i].rollback(); err != nil {
				runErr = errors.Join(runErr, &updateRollbackError{cause: fmt.Errorf("rollback update package: %w", err)})
			}
		}
	}()
	// Stage the complete set before exchanging any package. A staging failure
	// cannot leave an earlier candidate installed.
	for _, candidate := range prepared.candidates {
		if !candidate.storage {
			continue
		}
		if err := checkContext(ctx); err != nil {
			return result, err
		}
		swap, err := deps.stage(prepared.home, candidate.old.Name, candidate.old.Source.Kind == "file", candidate.item.pkg.Files)
		if err != nil {
			return result, updatePackageFailure("update-apply-failed", candidate.old.Name, "cannot stage candidate storage", err)
		}
		swaps = append(swaps, swap)
	}
	for _, swap := range swaps {
		if err := checkContext(ctx); err != nil {
			return result, err
		}
		if err := swap.apply(); err != nil {
			// packageSwap includes any failed internal restore in this error.
			var recovery *packageRollbackError
			if errors.As(err, &recovery) {
				return result, &updateRollbackError{cause: err}
			}
			return result, updateFailure("update-apply-failed", "cannot exchange candidate package storage", err)
		}
	}
	if err := deps.files(ctx, prepared.files); err != nil {
		var recovery *install.RollbackError
		if errors.As(err, &recovery) {
			return result, &updateRollbackError{cause: err}
		}
		return result, updateFailure("update-apply-failed", "cannot apply update files; completed file changes were rolled back", err)
	}
	for _, swap := range swaps {
		swap.commit()
	}
	committed = true
	result.Status, result.Changed, result.AwaitingConfirmation = "applied", true, false
	return result, nil
}
