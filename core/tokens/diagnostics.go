package tokens

import (
	"sort"

	"github.com/inxups/style-cli/core/spec"
)

func addSpecDiagnostics(report *Report, pkg *spec.ResolvedPackage, sourceDiagnostics []spec.Diagnostic) {
	for _, diagnostic := range sourceDiagnostics {
		switch diagnostic.Code {
		case "T001", "T005", "T006", "T007":
			addDiagnostic(report, Diagnostic{Code: diagnostic.Code, RuleID: ruleIDBySource(pkg, diagnostic.Path), Severity: diagnostic.Severity, Message: diagnostic.Message})
		}
	}
}

func ruleIDBySource(pkg *spec.ResolvedPackage, source string) string {
	for _, rule := range pkg.Rules {
		if rule.Source == source || rule.BodySource == source || rule.Source+"#model_text" == source {
			return ruleID(rule)
		}
	}
	return ""
}

func addDiagnostic(report *Report, diagnostic Diagnostic) {
	for _, existing := range report.Diagnostics {
		if existing.Code == diagnostic.Code && existing.RuleID == diagnostic.RuleID && existing.Event == diagnostic.Event {
			return
		}
	}
	report.Diagnostics = append(report.Diagnostics, diagnostic)
}

func sortDiagnostics(values []Diagnostic) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Code != values[j].Code {
			return values[i].Code < values[j].Code
		}
		if values[i].RuleID != values[j].RuleID {
			return values[i].RuleID < values[j].RuleID
		}
		return values[i].Event < values[j].Event
	})
}
