package app

import "context"

type TrustRequest struct {
	Home         string
	Name         string
	Yes          bool
	AllowReplace bool
	// Confirm receives a detached preview. All inputs and planned Before bytes
	// have already been captured; the subsequent commit never rereads sources.
	Confirm func(SyncResult) (bool, error)
}

// Trust previews source and compiled changes before approving exactly that
// snapshot. Without Yes or Confirm this is a read-only preview.
func (s *Service) Trust(ctx context.Context, request TrustRequest) (SyncResult, error) {
	if request.Name == "" {
		return SyncResult{}, &UsageError{Message: "package name is required"}
	}
	prepared, err := s.prepareSync(ctx, request)
	if err != nil {
		return prepared.result, err
	}
	confirmed := request.Yes
	if !confirmed && request.Confirm != nil {
		// Serialize the public value to keep callback mutation separate from
		// the transaction's summaries and captured byte slices.
		preview := detachedSyncResult(prepared.result)
		confirmed, err = request.Confirm(preview)
		if err != nil {
			return prepared.result, err
		}
	}
	if !confirmed {
		prepared.result.AwaitingConfirmation = true
		return prepared.result, nil
	}
	return applySync(ctx, prepared)
}
