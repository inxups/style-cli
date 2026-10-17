package agenthooks

import (
	"context"
	"fmt"
	"time"
)

// OfflineInputs replaces external codec supplements during an explicit replay.
// The default Runner continues using its normal discovery and Skill readers.
// Inputs contain already captured facts, never filenames or executable commands.
type OfflineInputs struct {
	SkillContent *string
	MCP          *MCPIdentity
}

type MCPIdentity struct {
	Server string
	Tool   string
}

// RunOutcome separates management failures from provider fail-open exit codes.
// It contains no payload, decisions, errors, or rendered text.
type RunOutcome struct {
	Stage      string
	ErrorCode  string
	Encoded    bool
	FailPolicy string
}

func WithOfflineInputs(inputs OfflineInputs) Option {
	return func(r *Runner) {
		clone := inputs
		if inputs.SkillContent != nil {
			v := *inputs.SkillContent
			clone.SkillContent = &v
		}
		if inputs.MCP != nil {
			v := *inputs.MCP
			clone.MCP = &v
		}
		r.offline = &clone
		r.mcpResolveOff = true
		r.mcpListOff = true
		r.backfillOff = true
		r.dedupOff = true
	}
}

// WithRunObserver observes one completed Run. The callback must not block.
func WithRunObserver(fn func(RunOutcome)) Option { return func(r *Runner) { r.runObserver = fn } }

// RecognizesNativeEvent uses the codec's own event table. Applications may
// separately register named OnOther handlers for their own supported events.
func RecognizesNativeEvent(provider Provider, name string) bool {
	switch provider {
	case ProviderCodex:
		_, ok := codexKinds[name]
		return ok
	case ProviderClaudeCode:
		_, ok := claudeKinds[name]
		return ok
	}
	return false
}

func (r *Runner) decodeOffline(provider Provider, variant Variant, confidence DetectionConfidence, payload []byte) (any, error) {
	switch provider {
	case ProviderClaudeCode:
		return decodeClaudeWithSkillReader(variant, confidence, time.Time{}, payload, func(string, string) (string, bool) {
			if r.offline.SkillContent == nil {
				return "", false
			}
			return *r.offline.SkillContent, true
		})
	case ProviderCodex:
		return decodeCodex(variant, confidence, time.Time{}, payload)
	default:
		return nil, fmt.Errorf("agenthooks: offline provider unsupported")
	}
}

func (r *Runner) applyOfflineMCP(typed any) error {
	if r.offline.MCP == nil {
		return nil
	}
	tool := toolOf(typed)
	if tool == nil {
		return fmt.Errorf("offline facts conflict")
	}
	if tool.MCP == nil && tool.Canonical != ToolMCP && tool.Canonical != ToolOther {
		return fmt.Errorf("offline facts conflict")
	}
	identity := tool.MCP
	if identity == nil {
		identity = ParseMCPName(tool.Name)
	}
	if identity == nil {
		identity = &MCPCall{}
	}
	provided := r.offline.MCP
	if identity.Server != "" && identity.Server != provided.Server || identity.Tool != "" && identity.Tool != provided.Tool {
		return fmt.Errorf("offline facts conflict")
	}
	clone := *identity
	if clone.Server == "" {
		clone.Server = provided.Server
	}
	if clone.Tool == "" {
		clone.Tool = provided.Tool
	}
	tool.MCP = &clone
	return nil
}

func (r *Runner) decideOffline(ctx context.Context, typed any) (d Decision, err error) {
	defer func() {
		if recover() != nil {
			d = nil
			err = fmt.Errorf("agenthooks: offline handler panic")
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	r.observe(ctx, typed)
	d, err = r.runPipeline(ctx, typed)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return d, err
}
