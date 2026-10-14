package engine

import (
	"strconv"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/when"
)

func matchesTarget(entry compile.Entry, point Point) bool {
	match := entry.Match
	switch entry.Target {
	case "event", "system":
		return match == nil
	case "tool":
		if match == nil {
			return false
		}
		for _, selector := range match.Canonical {
			if selector == "*" || selector == point.CanonicalTool {
				return true
			}
		}
		for _, selector := range match.Names {
			if selector == "*" || strings.EqualFold(selector, point.ToolName) {
				return true
			}
		}
		return false
	case "mcp":
		if match == nil {
			return false
		}
		for _, selector := range match.MCP {
			serverMatches := selector.Server == "*" || selector.Server == point.MCPServer
			toolMatches := selector.Tool == "*" || selector.Tool == point.MCPTool
			if serverMatches && toolMatches {
				return true
			}
		}
		return false
	case "skill":
		if match == nil {
			return false
		}
		for _, selector := range match.Skill {
			if selector == "*" || strings.EqualFold(selector, point.SkillName) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func matchesSource(entry compile.Entry, point, source string) bool {
	if point != "session.start" {
		return true
	}
	if entry.Sources == nil {
		return false
	}
	for _, allowed := range *entry.Sources {
		if allowed == source {
			return true
		}
	}
	return false
}

func mergeWhenInput(point when.Input, input Input) when.Input {
	merged := point
	if merged.Model == "" {
		merged.Model = input.Variables["model"]
	}
	if merged.ThinkingEffort == "" {
		merged.ThinkingEffort = input.Variables["thinking_effort"]
	}
	if merged.ThinkingEffortApprox == nil {
		if value, err := strconv.ParseBool(input.Variables["thinking_effort_approx"]); err == nil {
			merged.ThinkingEffortApprox = &value
		}
	}
	if merged.AgentID == "" && input.Scope.AgentID != "main" {
		merged.AgentID = input.Scope.AgentID
	}
	if merged.Harness == nil {
		merged.Harness = make(map[string]string)
	}
	for name, value := range input.Variables {
		if strings.HasPrefix(name, "harness.") {
			key := strings.TrimPrefix(name, "harness.")
			if _, exists := merged.Harness[key]; !exists {
				merged.Harness[key] = value
			}
		}
	}
	return merged
}

func mergeValues(base, point map[string]string, input when.Input) map[string]string {
	values := make(map[string]string, len(base)+len(point)+len(input.Harness)+4)
	for key, value := range base {
		values[key] = value
	}
	for key, value := range point {
		values[key] = value
	}
	if input.Model != "" {
		values["model"] = input.Model
	}
	if input.ThinkingEffort != "" {
		values["thinking_effort"] = input.ThinkingEffort
	}
	for key, value := range input.Harness {
		values["harness."+key] = value
	}
	if input.Fields == nil {
		return values
	}
	for _, name := range []string{
		"prompt", "tool.name", "tool.canonical", "tool.input", "tool.added", "tool.output",
		"mcp.server", "mcp.tool", "skill.name", "cwd",
	} {
		if value, ok := input.Fields.ReadConditionField(name); ok {
			values[name] = value
		}
	}
	return values
}

func isPromptPoint(point string) bool {
	return point == "prompt.submitted"
}
