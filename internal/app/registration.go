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
	for _, r := range reports {
		if r.State != "enabled" {
			return true
		}
	}
	return false
}
