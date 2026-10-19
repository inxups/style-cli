package app

import (
	"context"
	"encoding/json"
	"fmt"
)

// FrozenRequest restores the entire current lock. It has no source, settings,
// detection, route-selection or relocation inputs.
type FrozenRequest struct {
	Home              string
	Yes, AllowReplace bool
	Confirm           func(FrozenResult) (bool, error)
}

type FrozenContent struct {
	Package       string `json:"package"`
	Snapshot      string `json:"snapshot"`
	StorageRepair bool   `json:"storage_repair"`
}

type FrozenResult struct {
	UpdateResult
	LockSHA256   string          `json:"lock_sha256"`
	LockedSource bool            `json:"locked_source"`
	Contents     []FrozenContent `json:"contents"`
}

func newFrozenResult() FrozenResult {
	result := FrozenResult{UpdateResult: newUpdateResult(), LockedSource: true, Contents: []FrozenContent{}}
	result.Command = "restore"
	return result
}

// InstallFrozen verifies one captured, offline candidate against the complete
// lock and approval bytes. The lock, settings and approvals are read-only.
func (s *Service) InstallFrozen(ctx context.Context, request FrozenRequest) (result FrozenResult, runErr error) {
	result = newFrozenResult()
	defer func() {
		if runErr != nil {
			result.Status, result.AwaitingConfirmation = "error", false
			result.Diagnostics = append(result.Diagnostics, frozenDiagnostic(runErr))
			sortDiagnostics(result.Diagnostics)
		}
	}()
	prepared, err := s.prepareFrozen(ctx, request)
	result = prepared.result
	if err != nil {
		return result, err
	}
	if !prepared.transaction.changed() {
		return result, prepared.transaction.inputs.verify()
	}
	confirmed := request.Yes
	if !confirmed && request.Confirm != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return result, err
		}
		var preview FrozenResult
		if err := json.Unmarshal(data, &preview); err != nil {
			return result, err
		}
		confirmed, err = request.Confirm(preview)
		if err != nil {
			return result, updateFailure("confirmation-failed", "restore confirmation did not complete", err)
		}
	}
	if !confirmed {
		return result, nil
	}
	// Update's internal transaction stages every package first and performs one
	// guarded ApplyFiles, with typed rollback failures and retained backups.
	result.UpdateResult, err = applyUpdate(ctx, prepared.transaction, updateDependencies{})
	return result, err
}

func frozenDiagnostic(err error) Diagnostic {
	diagnostic := updateDiagnostic(err)
	switch diagnostic.Code {
	case "update-failed":
		diagnostic.Code = "frozen-failed"
		diagnostic.Message = "restore could not be completed; inspect the installation before retrying"
	case "canceled":
		diagnostic.Message = "restore was canceled; any incomplete installation changes were rolled back"
	case "installation-changed":
		diagnostic.Message = "captured installation inputs changed; rerun style restore to review a fresh recovery plan"
	case "update-apply-failed":
		diagnostic.Code = "frozen-apply-failed"
		diagnostic.Message = "cannot apply restore; inspect the installation before retrying"
	}
	return diagnostic
}

func frozenPackageFailure(code, name, action string, cause error) error {
	return updateFailure(code, fmt.Sprintf("package %q: %s", name, action), cause)
}

func frozenMetadata(message string, cause error) error {
	return updateFailure("frozen-metadata-incomplete", message+"; use the existing install/sync/trust flow to establish a complete baseline", cause)
}

func frozenKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// Recognition does not reinterpret values or paths as regular install inputs.
