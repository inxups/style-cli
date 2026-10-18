package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/source"
)

// UpdateRequest selects existing sources. It cannot change installation
// bindings, source URL/path/kind, or supply settings from outside the package.
type UpdateRequest struct {
	Home, Name, Ref string
	All             bool
	Yes             bool
	AllowReplace    bool
	Confirm         func(UpdateResult) (bool, error)
}

type UpdateSource struct {
	Kind string      `json:"kind"`
	Path string      `json:"path"`
	Link bool        `json:"link"`
	Git  *source.Git `json:"git,omitempty"`
}

type UpdateRevision struct {
	Source        UpdateSource `json:"source"`
	Version       string       `json:"version"`
	ContentSHA256 string       `json:"content_sha256"`
	Settings      []string     `json:"settings"`
	Replace       bool         `json:"replace"`
	LinkTrusted   bool         `json:"link_trusted"`
}

// Diffs and native commands belong to the explicit approval disclosure. Safe
// revisions and diagnostics contain no settings values or command bodies.
type UpdateChange struct {
	Package             string             `json:"package"`
	Sequence            int                `json:"seq"`
	Before              UpdateRevision     `json:"before"`
	After               *UpdateRevision    `json:"after,omitempty"`
	Acquisition         *SourceReport      `json:"acquisition,omitempty"`
	BaselineAvailable   bool               `json:"baseline_available"`
	SourceChanged       bool               `json:"source_changed"`
	RenderedChanged     bool               `json:"rendered_changed"`
	NativeChanged       bool               `json:"native_changed"`
	ConfigurationRepair bool               `json:"configuration_repair"`
	ApprovalChanged     bool               `json:"approval_changed"`
	SettingsAdded       []string           `json:"settings_added"`
	SettingsRemoved     []string           `json:"settings_removed"`
	SourceDiff          []FileDiff         `json:"source_diff,omitempty"`
	RenderedDiff        []FileDiff         `json:"rendered_diff,omitempty"`
	NativeDiff          []NativeHookDiff   `json:"native_diff,omitempty"`
	NativeBefore        []NativeHookReport `json:"native_before,omitempty"`
	NativeAfter         []NativeHookReport `json:"native_after,omitempty"`
}

type UpdatePlanReport struct {
	Package string `json:"package"`
	PlanReport
}

// UpdateResult is a single document for successful and failed operations.
// Changed describes committed installation writes, not cache acquisition.
type UpdateResult struct {
	Format               int                `json:"format"`
	Command              string             `json:"command"`
	Status               string             `json:"status"`
	Selected             []string           `json:"selected"`
	Changes              []UpdateChange     `json:"changes"`
	Reports              []UpdatePlanReport `json:"reports"`
	Diagnostics          []Diagnostic       `json:"diagnostics"`
	Routes               []string           `json:"routes"`
	Native               []NativeHookReport `json:"native_hooks,omitempty"`
	Changed              bool               `json:"changed"`
	AwaitingConfirmation bool               `json:"awaiting_confirmation"`
}

func newUpdateResult() UpdateResult {
	return UpdateResult{Format: 1, Command: "update", Status: "unchanged", Selected: []string{},
		Changes: []UpdateChange{}, Reports: []UpdatePlanReport{}, Diagnostics: []Diagnostic{}, Routes: []string{}}
}

// Update captures and validates one joint candidate, then commits exactly
// those bytes after confirmation. It never invokes sync/trust or Detect.
func (s *Service) Update(ctx context.Context, request UpdateRequest) (result UpdateResult, runErr error) {
	result = newUpdateResult()
	defer func() {
		if runErr != nil {
			result.Status, result.AwaitingConfirmation = "error", false
			result.Diagnostics = append(result.Diagnostics, updateDiagnostic(runErr))
			sortDiagnostics(result.Diagnostics)
		}
	}()
	prepared, err := s.prepareUpdate(ctx, request)
	result = prepared.result
	if err != nil {
		return result, err
	}
	if !prepared.changed() {
		return result, prepared.inputs.verify()
	}
	confirmed := request.Yes
	if !confirmed && request.Confirm != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return result, err
		}
		var preview UpdateResult
		if err := json.Unmarshal(data, &preview); err != nil {
			return result, err
		}
		confirmed, err = request.Confirm(preview)
		if err != nil {
			return result, updateFailure("confirmation-failed", "update confirmation did not complete", err)
		}
	}
	if !confirmed {
		return result, nil
	}
	return applyUpdate(ctx, prepared, updateDependencies{})
}

type updateError struct {
	code, message string
	cause         error
}

func (e *updateError) Error() string { return e.message }
func (e *updateError) Unwrap() error { return e.cause }
func updateFailure(code, message string, cause error) error {
	if spec.ErrorCode(cause) == spec.NativeFormatRemoved {
		return cause
	}
	return &updateError{code: code, message: message, cause: cause}
}

func updateDiagnostic(err error) Diagnostic {
	code, message := "update-failed", "update could not be completed; inspect the installation before retrying"
	var failure *updateError
	var remote *source.Error
	var blocked *BlockedError
	switch {
	case errors.As(err, new(*updateRollbackError)):
		code, message = "rollback-failed", err.Error()
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code, message = "canceled", "update was canceled; any incomplete installation changes were rolled back"
	case errors.Is(err, errManagementChanged):
		code, message = "installation-changed", "captured installation inputs changed; rerun update to review a fresh candidate"
	case errors.Is(err, errManagementBusy):
		code, message = "management-busy", "another management commit is active or its owner cannot be verified"
	case spec.ErrorCode(err) == spec.NativeFormatRemoved:
		code, message = spec.NativeFormatRemoved, err.Error()
	case errors.As(err, &remote):
		code, message = remote.Code, remote.Message
	case errors.As(err, &failure):
		code, message = failure.code, failure.message
	case errors.As(err, &blocked):
		code, message = "plan-blocked", "the joint update plan is blocked by fallback or budget policy"
		for _, budget := range []string{"T001", "T002", "T003", "T004"} {
			if strings.HasPrefix(blocked.Message, budget+" ") {
				code = budget
				break
			}
		}
	}
	return Diagnostic{Code: code, Severity: "error", Message: safeUpdateText(message)}
}

func safeUpdateText(text string) string {
	var out strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func updateRevision(item lockPackage) UpdateRevision {
	var git *source.Git
	if item.Source.Git != nil {
		copy := *item.Source.Git
		git = &copy
	}
	return UpdateRevision{Source: UpdateSource{Kind: item.Source.Kind, Path: item.Source.Path, Link: item.Link, Git: git},
		Version: item.Version, ContentSHA256: packageLockHash(item.Files), Settings: append([]string{}, item.Settings...),
		Replace: item.Replace, LinkTrusted: item.LinkTrusted}
}

func updatePackageFailure(code, name, action string, cause error) error {
	return updateFailure(code, fmt.Sprintf("package %q: %s", name, action), cause)
}
