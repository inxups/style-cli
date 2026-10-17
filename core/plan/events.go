package plan

func planEvents(req Requirement, result RulePlan, adapter Adapter) RulePlan {
	events := req.Events
	var explicit *eventResult
	if req.Target == "skill" {
		e := evaluate(req, adapter, SkillExplicit)
		explicit = &e
		events = events[:len(events)-1]
	}
	implicit := planHook(req, adapter, events)
	roles := []eventResult{implicit}
	if explicit != nil {
		roles = append(roles, *explicit)
	}

	unavailable := false
	allDataMissing := true
	for _, role := range roles {
		if role.usable {
			result.Placements = append(result.Placements, role.placement)
			result.Status = combineStatus(result.Status, role.placement.Status)
			allDataMissing = false
		} else {
			unavailable = true
			result.Reasons = append(result.Reasons, role.reason)
			allDataMissing = allDataMissing && role.dataMissing
			if req.Fallback == "error" {
				result.Fatal = true
			}
		}
	}
	if result.Fatal {
		result.Status = InstallError
		result.Placements = nil
		return result
	}
	if unavailable && req.Fallback == "skip" && !allDataMissing {
		return failedPlan(result, req, "fallback:skip removes the rule because one or more placements are unavailable")
	}
	if len(result.Placements) > 0 {
		if unavailable {
			result.Status = combineStatus(result.Status, Partial)
		}
		return result
	}
	if allDataMissing && req.Fallback != "error" {
		return neverPlan(result)
	}
	canStatic := req.Target == "tool" || req.Target == "mcp" || req.Target == "skill"
	// Runtime conditions cannot be preserved by an always-on instruction.
	if canStatic && req.Refresh == 0 && len(events) > 0 && req.Fallback == "degrade" && req.Action == "inject" && req.StaticBody && !req.HasWhen {
		if slot, supported := adapter.StaticLocation("append"); supported && slot != "" {
			result.Status = Degraded
			result.Placements = []Placement{{Action: "inject", Status: Degraded,
				StaticSlot: slot, StaticFallback: true, StaticPrefix: req.StaticPrefix,
				DegradedFrom: events[0]}}
			result.Reasons = append(result.Reasons, "using a static instruction qualified by this rule's target")
			return result
		}
	}
	return failedPlan(result, req, "no supported placement")
}

func planHook(req Requirement, adapter Adapter, events []string) eventResult {
	if len(events) == 0 {
		return eventResult{reason: "rule has no evaluation point"}
	}
	initial := evaluate(req, adapter, events[0])
	if initial.usable || req.Fallback != "degrade" || len(events) < 2 {
		return initial
	}
	alternate := evaluate(req, adapter, events[1])
	if alternate.usable {
		alternate.placement.Status = combineStatus(alternate.placement.Status, Degraded)
		alternate.placement.DegradedFrom = events[0]
		alternate.placement.Reasons = append(alternate.placement.Reasons, initial.reason)
	} else {
		alternate.reason = initial.reason + "; " + alternate.reason
		alternate.dataMissing = alternate.dataMissing && initial.dataMissing
	}
	return alternate
}
