package plan

func planSystem(req Requirement, result RulePlan, adapter Adapter) RulePlan {
	staticRule := req.Action == "inject" && req.StaticBody && !req.HasWhen
	if staticRule {
		if slot, supported := adapter.StaticLocation(req.Mode); supported && slot != "" {
			result.Status = Native
			result.Placements = []Placement{{Action: req.Action, Status: Native, StaticSlot: slot}}
			return result
		}
		if req.Mode == "replace" || req.Fallback != "degrade" {
			return failedPlan(result, req, "adapter has no static "+req.Mode+" location")
		}
		result.Reasons = append(result.Reasons, "static location unavailable; using dynamic system placements")
		return planStaticSystemFallback(req, result, adapter)
	} else if req.Mode == "replace" {
		// Hook context appends cannot implement replacement semantics.
		return failedPlan(result, req, "dynamic system context cannot replace static instructions")
	}

	start := evaluate(req, adapter, SessionStart)
	prompt := evaluate(req, adapter, PromptSubmitted)
	if start.dataMissing && prompt.dataMissing && req.Fallback != "error" {
		return neverPlan(result, start.reason, prompt.reason)
	}
	if start.usable && prompt.usable {
		prompt.placement.Reevaluate = true
		result.Placements = []Placement{start.placement, prompt.placement}
		result.Status = combineStatus(start.placement.Status, prompt.placement.Status)
		return result
	}
	if start.usable || prompt.usable {
		available, missing := start, prompt
		if !available.usable {
			available, missing = prompt, start
		}
		if req.Fallback != "degrade" {
			return failedPlan(result, req, missing.reason)
		}
		available.placement.Status = combineStatus(available.placement.Status, Degraded)
		if available.placement.Event == PromptSubmitted {
			available.placement.Reevaluate = true
			available.placement.Repeat = "session"
			available.placement.DegradedFrom = SessionStart
		} else {
			available.placement.DegradedFrom = PromptSubmitted
		}
		result.Placements = []Placement{available.placement}
		result.Status = available.placement.Status
		result.Reasons = append(result.Reasons, missing.reason)
		return result
	}
	result.Reasons = append(result.Reasons, start.reason, prompt.reason)
	if req.Fallback == "skip" {
		return failedPlan(result, req, "no supported system placement")
	}
	// A dynamic system rule must not silently disappear when there is no
	// usable hook or static location (spec section 5, step 2).
	result.Status, result.Fatal = InstallError, true
	return result
}

// planStaticSystemFallback follows the runtime rule's explicit degradation
// order: use session.start first, then prompt.submitted with session repeat.
// It deliberately emits one placement so a static system rule is not injected
// twice on the same session.
func planStaticSystemFallback(req Requirement, result RulePlan, adapter Adapter) RulePlan {
	start := evaluate(req, adapter, SessionStart)
	if start.usable {
		start.placement.Status = combineStatus(start.placement.Status, Degraded)
		start.placement.DegradedFrom = "static"
		result.Placements = []Placement{start.placement}
		result.Status = start.placement.Status
		return result
	}
	prompt := evaluate(req, adapter, PromptSubmitted)
	if prompt.usable {
		prompt.placement.Status = combineStatus(prompt.placement.Status, Degraded)
		prompt.placement.Reevaluate = true
		prompt.placement.Repeat = "session"
		prompt.placement.DegradedFrom = SessionStart
		result.Placements = []Placement{prompt.placement}
		result.Status = prompt.placement.Status
		result.Reasons = append(result.Reasons, start.reason)
		return result
	}
	if start.dataMissing && prompt.dataMissing && req.Fallback != "error" {
		return neverPlan(result, start.reason, prompt.reason)
	}
	result.Reasons = append(result.Reasons, start.reason, prompt.reason)
	if req.Fallback == "skip" {
		return failedPlan(result, req, "no usable dynamic fallback for static system rule")
	}
	result.Status, result.Fatal = InstallError, true
	return result
}
