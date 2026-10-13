package tokens

import (
	"fmt"
	"sort"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
)

const (
	DefaultPerRule  = 400
	DefaultPerEvent = 1200
	DefaultAlwaysOn = 600
)

// Analyze estimates the model-facing output selected by a plan. The package
// must be merged and the plan must contain the same rules in source order.
// Analyze estimates model-facing output for a merged package and its plan.
// When source diagnostics are omitted, shared STYLE lints are evaluated here.
func Analyze(pkg *spec.ResolvedPackage, planned plan.Report, sourceDiagnostics ...spec.Diagnostic) (Report, error) {
	return AnalyzeWithLayout(pkg, planned, UnifiedLayout{}, sourceDiagnostics...)
}

// AnalyzeWithLayout is Analyze with an adapter-supplied native event/tool map.
func AnalyzeWithLayout(pkg *spec.ResolvedPackage, planned plan.Report, layout Layout, sourceDiagnostics ...spec.Diagnostic) (Report, error) {
	if pkg == nil || pkg.Package == nil {
		return Report{}, fmt.Errorf("token analysis requires a resolved package")
	}
	if len(pkg.Rules) != len(planned.Rules) {
		return Report{}, fmt.Errorf("token analysis requires one plan entry per resolved rule")
	}
	if layout == nil {
		return Report{}, fmt.Errorf("token analysis requires a layout")
	}
	if len(sourceDiagnostics) == 0 {
		sourceDiagnostics = spec.Validate(pkg.Package)
	}
	limits := limitsFromPackage(pkg)
	report := Report{Rules: make([]RuleEstimate, 0, len(pkg.Rules))}
	eventTotals := make(map[string]*eventAccumulator)
	for index, rule := range pkg.Rules {
		plannedRule := planned.Rules[index]
		if plannedRule.ID == "" || plannedRule.ID != ruleID(rule) {
			return Report{}, fmt.Errorf("plan rule %d does not match resolved rule", index)
		}
		requirement, err := plan.RequirementFor(rule)
		if err != nil {
			return Report{}, err
		}
		ruleEstimate, placements, err := estimateRule(rule, plannedRule, pkg)
		if err != nil {
			return Report{}, err
		}
		report.Rules = append(report.Rules, ruleEstimate)
		maxTokens := nonnegativeLimit(rule.Effective["max_tokens"], limits.PerRule)
		if float64(ruleEstimate.Estimate.Tokens) > maxTokens {
			addDiagnostic(&report, Diagnostic{Code: "T001", RuleID: ruleEstimate.RuleID, Severity: budgetSeverity(pkg), Message: fmt.Sprintf("rule estimate %d exceeds max_tokens %s", ruleEstimate.Estimate.Tokens, formatLimit(maxTokens))})
		}
		for _, placement := range placements {
			if isAlwaysOn(placement.placement, requirement) {
				report.AlwaysOn = add(report.AlwaysOn, placement.estimate)
			}
			if placement.placement.Event == "" {
				continue
			}
			buckets := uniqueBuckets(layout.Buckets(rule, placement.placement))
			if len(buckets) == 0 {
				return Report{}, fmt.Errorf("budget layout has no native bucket for rule %q at %s", ruleEstimate.RuleID, placement.placement.Event)
			}
			for _, bucket := range buckets {
				accumulator := eventTotals[bucket.Key]
				if accumulator == nil {
					accumulator = &eventAccumulator{label: nonEmpty(bucket.Label, bucket.Key)}
					eventTotals[bucket.Key] = accumulator
				} else if bucket.Label != "" && bucket.Label < accumulator.label {
					accumulator.label = bucket.Label
				}
				accumulator.add(placement.placement.Action, placement.estimate)
			}
		}
	}
	for key, accumulator := range eventTotals {
		estimate := accumulator.worstCase()
		report.Events = append(report.Events, EventEstimate{
			Key: key, Label: accumulator.label, Estimate: estimate,
			Block: accumulator.block, Other: accumulator.other, HasBlock: accumulator.hasBlock,
			BlockCount: accumulator.blockCount, OtherCount: accumulator.otherCount,
		})
		if float64(estimate.Tokens) > limits.PerEvent {
			addDiagnostic(&report, Diagnostic{Code: "T002", Event: accumulator.label, Severity: budgetSeverity(pkg), Message: fmt.Sprintf("event estimate %d exceeds per_event %s", estimate.Tokens, formatLimit(limits.PerEvent))})
		}
	}
	sort.Slice(report.Events, func(i, j int) bool { return report.Events[i].Key < report.Events[j].Key })
	if float64(report.AlwaysOn.Tokens) > limits.AlwaysOn {
		addDiagnostic(&report, Diagnostic{Code: "T004", Severity: budgetSeverity(pkg), Message: fmt.Sprintf("always-on estimate %d exceeds always_on %s", report.AlwaysOn.Tokens, formatLimit(limits.AlwaysOn))})
	}
	addSpecDiagnostics(&report, pkg, sourceDiagnostics)
	sortDiagnostics(report.Diagnostics)
	return report, nil
}
