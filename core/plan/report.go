package plan

import (
	"fmt"
	"strings"
)

// String renders a stable human-readable report. It never includes rule bodies
// or template values.
func (report Report) String() string {
	var output strings.Builder
	if report.Package != "" {
		fmt.Fprintf(&output, "Package: %s\n", report.Package)
	}
	fmt.Fprintf(&output, "Adapter: %s\n", report.Adapter)
	for _, rule := range report.Rules {
		if rule.Action == "" {
			fmt.Fprintf(&output, "- %s: %s (%s)\n", rule.ID, rule.Status, rule.Target)
		} else {
			fmt.Fprintf(&output, "- %s: %s (%s %s)\n", rule.ID, rule.Status, rule.Action, rule.Target)
		}
		if len(rule.Needs) > 0 {
			fmt.Fprintf(&output, "  needs: %s\n", strings.Join(rule.Needs, ", "))
		}
		if rule.Refresh != nil {
			fmt.Fprintf(&output, "  refresh: every %d submitted turns since last emission; first match immediately; matched points only; full per-emission budget\n", rule.Refresh.Interval)
			for _, limitation := range rule.Refresh.Limits {
				fmt.Fprintf(&output, "     %s\n", limitation)
			}
		}
		if rule.DeduplicatedInto != "" {
			fmt.Fprintf(&output, "  deduplicated into %s; no additional cost\n", rule.DeduplicatedInto)
		}
		for _, placement := range rule.Placements {
			destination := placement.Event
			if placement.StaticSlot != "" {
				destination = "static:" + placement.StaticSlot
			}
			fmt.Fprintf(&output, "  -> %s %s (%s)", destination, placement.Action, placement.Status)
			if len(placement.Capabilities) > 0 {
				capabilities := make([]string, len(placement.Capabilities))
				for i, capability := range placement.Capabilities {
					capabilities[i] = string(capability)
				}
				fmt.Fprintf(&output, "; capabilities: %s", strings.Join(capabilities, ", "))
			}
			output.WriteByte('\n')
			for _, reason := range placement.Reasons {
				fmt.Fprintf(&output, "     %s\n", reason)
			}
			for _, warning := range placement.Warnings {
				fmt.Fprintf(&output, "     warning: %s\n", warning)
			}
		}
		for _, reason := range rule.Reasons {
			fmt.Fprintf(&output, "  ! %s\n", reason)
		}
	}
	if len(report.NativeHooks) > 0 {
		output.WriteString("Native hooks:\n")
	}
	for _, hook := range report.NativeHooks {
		fmt.Fprintf(&output, "- %s: %s (native)\n", hook.ID, hook.Status)
		if hook.Native != nil {
			fmt.Fprintf(&output, "  -> native %s", hook.Native.Event)
			if hook.Native.Matcher != "" {
				fmt.Fprintf(&output, " matcher %s", hook.Native.Matcher)
			}
			fmt.Fprintf(&output, "; timeout %ds\n", hook.Native.Timeout)
		}

		for _, reason := range hook.Reasons {
			fmt.Fprintf(&output, "  ! %s\n", reason)
		}
	}
	if report.Blocked {
		output.WriteString("Plan rejected: one or more rules prevent installation.\n")
	}
	return output.String()
}
