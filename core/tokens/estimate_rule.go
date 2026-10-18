package tokens

import (
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/warn"
	"github.com/inxups/style-cli/internal/tokenestimate"
)

// EstimatePlacement estimates exactly one plan placement's model-facing text.
// Unsupported or user-only output has zero model cost. Compilation uses the
// same policy as Analyze so index estimates cannot diverge from plan reports.
func EstimatePlacement(rule spec.Rule, placement plan.Placement, pkg *spec.ResolvedPackage) (Estimate, error) {
	text, include, err := modelText(rule, placement, pkg)
	if err != nil || !include {
		return Estimate{}, err
	}
	return EstimateText(text), nil
}

func estimateRule(rule spec.Rule, planned plan.RulePlan, pkg *spec.ResolvedPackage) (RuleEstimate, []placementEstimate, error) {
	result := RuleEstimate{RuleID: planned.ID, Action: planned.Action, Status: planned.Status}
	result.DeduplicatedInto = planned.DeduplicatedInto
	placements := make([]placementEstimate, 0, len(planned.Placements))
	for _, placement := range planned.Placements {
		text, include, err := modelText(rule, placement, pkg)
		if err != nil {
			return RuleEstimate{}, nil, err
		}
		if !include {
			continue
		}
		count := tokenestimate.CountText(text)
		estimate := Estimate{Tokens: count.Tokens, Chars: count.Chars}
		if estimate.Tokens > result.Estimate.Tokens {
			result.Estimate = estimate
		}
		if placement.Event != "" {
			result.Events = append(result.Events, placement.Event)
		}
		placements = append(placements, placementEstimate{placement: placement, estimate: estimate})
	}
	result.Events = uniqueSorted(result.Events)
	return result, placements, nil
}

func modelText(rule spec.Rule, placement plan.Placement, pkg *spec.ResolvedPackage) (string, bool, error) {
	if placement.Status == plan.Skipped || placement.Status == plan.Never || placement.Status == plan.InstallError {
		return "", false, nil
	}
	if placement.Action == "warn" && !hasCapability(placement.Capabilities, plan.AddContext) {
		return "", false, nil
	}
	if placement.Action == "block" && placement.Event != plan.ToolPre {
		// Prompt-block reasons are shown to the user but do not enter model input.
		return "", true, nil
	}
	text := rule.Body
	if placement.Action == "warn" {
		if effectiveAction(rule, pkg) == "warn" {
			if value, ok := effectiveValue(rule, pkg, "model_text"); ok {
				var isString bool
				text, isString = value.(string)
				if !isString {
					return "", false, nil
				}
			} else {
				rendered, err := tokenestimate.RenderForEstimate(text, contextFor(pkg))
				if err != nil {
					return "", false, err
				}
				return warn.FirstSentence(rendered), true, nil
			}
		} else {
			rendered, err := tokenestimate.RenderForEstimate(text, contextFor(pkg))
			if err != nil {
				return "", false, err
			}
			return warn.FirstSentence(rendered), true, nil
		}
	}
	rendered, err := tokenestimate.RenderForEstimate(text, contextFor(pkg))
	if err != nil {
		return "", false, err
	}
	if placement.StaticFallback {
		rendered = placement.StaticPrefix + rendered
	}
	return rendered, true, nil
}

func hasCapability(values []plan.Capability, want plan.Capability) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
