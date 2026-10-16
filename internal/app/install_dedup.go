package app

import (
	"fmt"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
)

// effectiveInstallReports runs only after source budgets have passed and the
// final compiled placement set has been selected. Reports and T003 use that
// same selection; nothing reads source files again between these phases.
func effectiveInstallReports(result *installCompilation, packages []installPackage, adapters []installAdapter, compiled compile.Result, reportPackage string) error {
	type key struct{ harness, rule string }
	aliases := make(map[key]string)
	warnings := make(map[key]string)
	participants := make(map[key]bool)
	for _, relation := range compiled.Deduplicated {
		aliases[key{relation.Harness, relation.Rule}] = relation.Into
		for _, ref := range []string{relation.Rule, relation.Into} {
			pkg, _, _ := strings.Cut(ref, "/")
			participants[key{relation.Harness, pkg}] = true
		}
	}
	for _, warning := range compiled.DeduplicationWarnings {
		warnings[key{warning.Harness, warning.Rule}] = warning.Reason
		pkg, _, _ := strings.Cut(warning.Rule, "/")
		participants[key{warning.Harness, pkg}] = true
	}
	for _, target := range adapters {
		var reports []tokens.Report
		for index := range packages {
			item := &packages[index]
			planned, ok := item.report[target.provider]
			if !ok {
				continue
			}
			// Clone the slice before annotating: compile inputs retain their
			// independent single-package plans.
			planned.Rules = append(planned.Rules[:0:0], planned.Rules...)
			for i := range planned.Rules {
				rule := &planned.Rules[i]
				lookup := key{string(target.provider), item.lock.Name + "/" + rule.ID}
				rule.DeduplicatedInto = aliases[lookup]
				if reason := warnings[lookup]; reason != "" {
					rule.Reasons = append(append([]string(nil), rule.Reasons...), reason)
				}
			}
			budget, err := tokens.AnalyzeWithLayout(item.resolved, planned, layoutFor(target.provider), spec.Validate(item.pkg)...)
			if err != nil {
				return fmt.Errorf("effective token analysis %s/%s: %w", item.lock.Name, target.provider, err)
			}
			for i := range budget.Diagnostics {
				budget.Diagnostics[i].Message = "single-package " + item.lock.Name + ": " + budget.Diagnostics[i].Message
			}
			item.report[target.provider], item.tokens[target.provider] = planned, budget
			reports = append(reports, budget)
			if reportPackage != "" && (item.lock.Name == reportPackage || participants[key{string(target.provider), item.lock.Name}]) {
				result.reports = append(result.reports, installPlanReport(target, planned, budget, item.resolved))
			}
		}
		findings, err := tokens.AnalyzeInstalled(reports, target.definition.Limits.Input)
		if err != nil {
			return err
		}
		if len(findings) > 0 {
			finding := findings[0]
			return &BlockedError{Message: fmt.Sprintf("%s %s blocks install: %s", finding.Code, finding.Event, finding.Message)}
		}
	}
	return nil
}
