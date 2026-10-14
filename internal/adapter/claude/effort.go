// Package claude contains Claude Code-specific runtime data resolution.
package claude

import (
	"embed"
	"fmt"
	"os"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter/effortmap"
)

//go:embed effort.yaml
var effortFiles embed.FS

// EffortTable loads the provider's checked-in, validated effort mapping.
func EffortTable() (effortmap.Table, error) {
	file, err := effortFiles.Open("effort.yaml")
	if err != nil {
		return effortmap.Table{}, err
	}
	defer file.Close()
	return effortmap.Parse(file)
}

// EffortResolution records the selected source alongside a mapped value.
type EffortResolution struct {
	effortmap.Value
	Source string
}

// ResolveEffort applies Claude's event > environment > session-state priority.
// An unrecognized but reported event value remains authoritative and is kept
// as raw data rather than silently replaced with a lower-priority value.
func ResolveEffort(eventLevel, environment string, previous state.Session) (EffortResolution, error) {
	table, err := EffortTable()
	if err != nil {
		return EffortResolution{}, fmt.Errorf("claude effort: %w", err)
	}
	switch {
	case eventLevel != "":
		return EffortResolution{Value: table.Resolve(eventLevel), Source: "event"}, nil
	case environment != "":
		return EffortResolution{Value: table.Resolve(environment), Source: "environment"}, nil
	case previous.EffortRaw != "" || previous.ThinkingEffort != "":
		return EffortResolution{Value: effortmap.Value{
			Effort: previous.ThinkingEffort, EffortRaw: previous.EffortRaw,
			EffortApprox: cloneBool(previous.EffortApprox),
		}, Source: "session"}, nil
	default:
		return EffortResolution{}, nil
	}
}

// ResolveCurrentEffort is the hook-facing convenience form. It reads only
// CLAUDE_EFFORT for the environment fallback; callers should pass the event's
// effort.level and the already persisted session state.
func ResolveCurrentEffort(eventLevel string, previous state.Session) (EffortResolution, error) {
	return ResolveEffort(eventLevel, os.Getenv("CLAUDE_EFFORT"), previous)
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
