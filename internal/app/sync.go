package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/speakeasy-api/agenthooks/install"
)

type SyncRequest struct{ Home string }

type PackageChange struct {
	Package           string           `json:"package"`
	Link              bool             `json:"link"`
	Files             []string         `json:"files"`
	NeedsTrust        bool             `json:"needs_trust"`
	BaselineAvailable bool             `json:"baseline_available"`
	SourceDiff        []FileDiff       `json:"source_diff,omitempty"`
	RenderedDiff      []FileDiff       `json:"rendered_diff,omitempty"`
	NativeChanged     bool             `json:"native_changed,omitempty"`
	NativeDiff        []NativeHookDiff `json:"native_diff,omitempty"`
}

type SyncResult struct {
	ApprovalTarget       string             `json:"approval_target,omitempty"`
	Changed              bool               `json:"changed"`
	Changes              []PackageChange    `json:"changes"`
	NeedsTrust           []string           `json:"needs_trust"`
	AwaitingConfirmation bool               `json:"awaiting_confirmation"`
	Routes               []string           `json:"routes"`
	Reports              []PlanReport       `json:"reports,omitempty"`
	Diagnostics          []Diagnostic       `json:"diagnostics,omitempty"`
	ExpiredSessions      int                `json:"expired_sessions"`
	Native               []NativeHookReport `json:"native_hooks,omitempty"`
}

type syncPreparation struct {
	home   string
	result SyncResult
	files  []install.PlannedFile
}

// Sync blocks unapproved ordinary edits and the first development-link trust.
// Approved links may change ordinary rules freely. Native changes and native
// configuration repairs require renewed approval.
func (s *Service) Sync(ctx context.Context, request SyncRequest) (SyncResult, error) {
	prepared, err := s.prepareSync(ctx, TrustRequest{Home: request.Home})
	if err != nil {
		return prepared.result, err
	}
	if len(prepared.result.NeedsTrust) > 0 {
		return prepared.result, nil
	}
	return applySync(ctx, prepared)
}

func applySync(ctx context.Context, prepared syncPreparation) (SyncResult, error) {
	if err := checkContext(ctx); err != nil {
		return prepared.result, err
	}
	if err := install.ApplyFiles(ctx, prepared.files); err != nil {
		return prepared.result, fmt.Errorf("apply sync changes: %w", err)
	}
	prepared.result.Changed = len(prepared.files) > 0
	for i := range prepared.result.Changes {
		if prepared.result.Changes[i].Package == prepared.result.ApprovalTarget {
			prepared.result.Changes[i].NeedsTrust = false
		}
	}
	count, err := maintainState(filepath.Join(prepared.home, "state"), false)
	prepared.result.ExpiredSessions = count
	if err != nil {
		// The installation transaction has succeeded. Cleanup is maintenance,
		// so disclose a failure without misreporting the committed update.
		prepared.result.Diagnostics = append(prepared.result.Diagnostics, Diagnostic{Code: "state-cleanup", Severity: "warning", Message: err.Error()})
	}
	return prepared.result, nil
}
