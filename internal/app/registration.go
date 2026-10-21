package app

import (
	"context"
	"github.com/inxups/style-cli/internal/adapter"
)

func registrations(ctx context.Context, targets []installAdapter) ([]adapter.RegistrationReport, error) {
	var reports []adapter.RegistrationReport
	for _, target := range targets {
		if inspector, ok := target.route.(adapter.RegistrationInspector); ok {
			report, err := inspector.Registration(ctx, nil)
			if err != nil {
				return nil, err
			}
			if report != nil {
				reports = append(reports, *report)
			}
		}
	}
	return reports, nil
}
func registrationRequired(reports []adapter.RegistrationReport) bool {
	for _, d := range registrationDiagnostics(reports) {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

// registrationDiagnostics explains why an install that generated a plugin
// still exits 1: the harness loads it only after its native registration.
// Advisory routes keep static rules active, so they only warn.
func registrationDiagnostics(reports []adapter.RegistrationReport) []Diagnostic {
	var diagnostics []Diagnostic
	for _, r := range reports {
		if r.State != "enabled" {
			diagnostics = append(diagnostics, Diagnostic{Code: "registration-required", Severity: registrationSeverity(r), Message: r.Route + ": " + r.State + "; run the listed native registration commands"})
		}
	}
	return diagnostics
}

func registrationSeverity(r adapter.RegistrationReport) string {
	if r.Advisory {
		return "warning"
	}
	return "error"
}
