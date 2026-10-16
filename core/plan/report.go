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
		if rule.Native != nil {
			fmt.Fprintf(&output, "  -> native %s", rule.Native.Event)
			if rule.Native.Matcher != "" {
				fmt.Fprintf(&output, " matcher %s", rule.Native.Matcher)
			}
			fmt.Fprintf(&output, "; timeout %ds\n", rule.Native.Timeout)
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
	if report.Blocked {
		output.WriteString("Plan rejected: one or more rules prevent installation.\n")
	}
	return output.String()
}
