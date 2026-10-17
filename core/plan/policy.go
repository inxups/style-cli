package plan

import (
	"slices"
	"strings"
)

type eventResult struct {
	placement   Placement
	usable      bool
	dataMissing bool
	reason      string
}

func evaluate(req Requirement, adapter Adapter, event string) eventResult {
	p := Placement{Event: event, Action: req.Action, Repeat: req.Repeat, Status: Native}
	hard := make(map[string]bool, len(req.ConditionNeeds))
	for _, need := range req.ConditionNeeds {
		hard[need] = true
		if !adapter.DataSource(event, need) {
			p.MissingData = append(p.MissingData, need)
		}
	}
	for _, need := range req.Needs {
		if !hard[need] && !adapter.DataSource(event, need) {
			if slices.Contains(req.UnknownNeeds, need) {
				p.Warnings = append(p.Warnings, "runtime value "+need+" is unavailable; its explicit unknown selector may match")
			} else {
				p.Warnings = append(p.Warnings, "runtime value "+need+" is unavailable; its template default or empty text will be used")
			}
		}
	}
	if len(p.MissingData) > 0 {
		return eventResult{placement: p, dataMissing: true,
			reason: "missing condition data at " + event + ": " + strings.Join(p.MissingData, ", ")}
	}
	action := req.Action
	all, any := capabilitiesFor(action)
	available, missing := availableCapabilities(adapter, event, all, any)
	if len(missing) > 0 && action == "block" && req.Fallback == "degrade" {
		action, p.Action, p.Repeat, p.Status = "warn", "warn", "every", Degraded
		p.Reasons = append(p.Reasons, "block is unavailable; downgraded to warn")
		all, any = capabilitiesFor(action)
		available, missing = availableCapabilities(adapter, event, all, any)
	}
	if len(missing) > 0 {
		return eventResult{placement: p, reason: strings.Join(missing, "; ") + " at " + event}
	}
	p.Capabilities = available
	if len(any) > 0 && len(available) < len(any) {
		if req.Fallback != "degrade" {
			return eventResult{placement: p, reason: "warn output is partial at " + event}
		}
		p.Status = combineStatus(p.Status, Partial)
		p.Reasons = append(p.Reasons, "only one warn output capability is supported")
	}
	if req.Action == "block" && p.Action == "block" {
		p.Repeat = ""
	}
	return eventResult{placement: p, usable: true}
}

func availableCapabilities(adapter Adapter, event string, all, any []Capability) (available []Capability, missing []string) {
	for _, capability := range all {
		if !adapter.Supports(event, capability) {
			missing = append(missing, "missing "+string(capability))
			continue
		}
		available = append(available, capability)
	}
	for _, capability := range any {
		if adapter.Supports(event, capability) {
			available = append(available, capability)
		}
	}
	if len(any) > 0 && len(available) == 0 {
		missing = append(missing, "missing all of "+strings.Join(capabilityNames(any), " or "))
	}
	return available, missing
}

func capabilityNames(capabilities []Capability) []string {
	names := make([]string, len(capabilities))
	for i, capability := range capabilities {
		names[i] = string(capability)
	}
	return names
}

func failedPlan(result RulePlan, req Requirement, reason string) RulePlan {
	result.Reasons = append(result.Reasons, reason)
	result.Placements = nil
	result.Status = Skipped
	if req.Fallback == "error" {
		result.Status, result.Fatal = InstallError, true
	}
	return result
}

func neverPlan(result RulePlan, reasons ...string) RulePlan {
	result.Status = Never
	result.Placements = nil
	result.Reasons = append(result.Reasons, reasons...)
	return result
}

func combineStatus(a, b Status) Status {
	if a == "" {
		return b
	}
	if a == Partial || b == Partial {
		return Partial
	}
	if a == Degraded || b == Degraded {
		return Degraded
	}
	return Native
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
