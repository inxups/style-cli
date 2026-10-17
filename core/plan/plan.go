package plan

import (
	"fmt"
	"strings"

	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/state"
)

// Build plans an already-validated, merged package for one adapter. Adapters
// supply facts; fallback policy remains in core. Blocked reports must not be
// installed, even if some rules have supported placements.
func Build(pkg *spec.ResolvedPackage, adapter Adapter) (Report, error) {
	if pkg == nil || pkg.Package == nil {
		return Report{}, fmt.Errorf("plan requires a resolved package")
	}
	if adapter == nil {
		return Report{}, fmt.Errorf("plan requires an adapter")
	}
	adapterName := adapter.Name()
	if strings.TrimSpace(adapterName) == "" {
		return Report{}, fmt.Errorf("adapter name must not be empty")
	}
	report := Report{
		Package: stringValue(pkg.Package.Metadata["name"]),
		Adapter: adapterName,
		Rules:   make([]RulePlan, 0, len(pkg.Rules)),
	}
	for _, rule := range pkg.Rules {
		planned, err := planRule(rule, adapterName, adapter)
		if err != nil {
			return Report{}, err
		}
		report.Rules = append(report.Rules, planned)
		report.Blocked = report.Blocked || planned.Fatal
	}
	return report, nil
}

func planRule(rule spec.Rule, adapterName string, adapter Adapter) (RulePlan, error) {
	req, err := RequirementFor(rule)
	if err != nil {
		return RulePlan{}, err
	}
	result := RulePlan{ID: req.RuleID, Source: rule.Source, Target: req.Target,
		Action: req.Action, Needs: append([]string(nil), req.Needs...)}
	switch {
	case req.Target == "native":
		result = planNative(rule, req, result, adapter)
	case !harnessSelectorMatches(rule, adapterName):
		result.Status = Skipped
		result.Reasons = []string{"when.harness.name does not include this adapter"}
	case req.ProviderCondition:
		result.Status = Never
		result.Reasons = []string{"when.provider is not evaluated in M1"}
	case req.Refresh > 0 && !state.RefreshSupported():
		result = failedPlan(result, req, "refresh-unsupported-platform: refresh session locking is unavailable on this platform")
	default:
		reason, blind := adapter.BlindSpot(rule)
		if blind {
			result.Status = Never
			result.Reasons = []string{nonEmpty(reason, "the adapter cannot observe this target")}
		} else {
			if reason != "" {
				result.Reasons = append(result.Reasons, reason)
			}
			if req.Target == "system" {
				result = planSystem(req, result, adapter)
			} else {
				result = planEvents(req, result, adapter)
			}
		}
	}
	if req.Refresh > 0 {
		result.Refresh = DescribeRefresh(req.Refresh)
	}
	return result, nil
}
