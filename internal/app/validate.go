package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
)

// Diagnostic is the command-facing form of a parse, schema, lint, or plan
// finding. It keeps the stable code separate from human-readable wording.
type Diagnostic struct {
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Provider string `json:"provider,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// ValidateRequest selects the source package to inspect.
type ValidateRequest struct {
	Source string
}

// Validation contains the diagnostics and, when loading succeeded, the
// parsed package used by plan.
type Validation struct {
	Package     *spec.Package
	Diagnostics []Diagnostic
	source      []spec.Diagnostic
}

// Valid reports whether no error-severity finding was produced.
func (result Validation) Valid() bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == spec.SeverityError {
			return false
		}
	}
	return true
}

// Validate loads a package and runs schema validation plus the provider
// independent token budget checks that can be derived offline.
func (s *Service) Validate(ctx context.Context, request ValidateRequest) (Validation, error) {
	if err := checkContext(ctx); err != nil {
		return Validation{}, err
	}
	if strings.TrimSpace(request.Source) == "" {
		request.Source = "."
	}
	pkg, err := spec.Load(request.Source)
	if err != nil {
		return Validation{Diagnostics: []Diagnostic{parseDiagnostic(err)}}, nil
	}
	source := s.validateSpec(pkg)
	result := Validation{Package: pkg, Diagnostics: convertDiagnostics(source), source: source}
	if hasErrors(result.Diagnostics) {
		return result, nil
	}
	resolved, err := spec.Merge(pkg)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, parseDiagnostic(err))
		return result, nil
	}
	for _, provider := range s.providers() {
		definition := s.definitions[provider]
		planner, err := adapter.BuildPlanAdapter(provider, definition.Facts)
		if err != nil {
			continue
		}
		planned, err := plan.Build(resolved, planner)
		if err != nil || planned.Blocked {
			continue
		}
		budget, err := tokens.AnalyzeWithLayout(resolved, planned, layoutFor(provider), source...)
		if err != nil {
			continue
		}
		for _, finding := range budget.Diagnostics {
			if finding.Code != "T002" && finding.Code != "T004" {
				continue
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code: finding.Code, Rule: finding.RuleID, Provider: string(provider), Severity: finding.Severity, Message: finding.Message,
			})
		}
	}
	result.Diagnostics = uniqueDiagnostics(result.Diagnostics)
	sortDiagnostics(result.Diagnostics)
	return result, nil
}

func parseDiagnostic(err error) Diagnostic {
	code := spec.ErrorCode(err)
	if code == "" {
		code = "invalid-package"
	}
	return Diagnostic{Code: code, Severity: spec.SeverityError, Message: err.Error()}
}

func convertDiagnostics(values []spec.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(values))
	for _, value := range values {
		result = append(result, Diagnostic{Code: value.Code, Path: value.Path, Severity: value.Severity, Message: value.Message})
	}
	return result
}

func hasErrors(values []Diagnostic) bool {
	for _, value := range values {
		if value.Severity == spec.SeverityError {
			return true
		}
	}
	return false
}

func uniqueDiagnostics(values []Diagnostic) []Diagnostic {
	seen := make(map[string]bool, len(values))
	result := make([]Diagnostic, 0, len(values))
	for _, value := range values {
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s", value.Code, value.Path, value.Rule, value.Provider, value.Severity, value.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func sortDiagnostics(values []Diagnostic) {
	sort.SliceStable(values, func(i, j int) bool {
		a, b := values[i], values[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Message < b.Message
	})
}
