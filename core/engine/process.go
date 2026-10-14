package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
)

// Process evaluates all points in one provider call, applies repeat claims,
// gives block decisions precedence, merges output, and releases capped claims.
func (e *Engine) Process(input Input, limit tokens.InputLimit) (Output, error) {
	if e == nil || e.store == nil {
		return Output{}, fmt.Errorf("engine: uninitialized engine")
	}
	if input.Harness == "" || input.Scope.SessionID == "" || input.Scope.AgentID == "" {
		return Output{}, fmt.Errorf("engine: harness and session scope are required")
	}
	if limit.Value < 0 || (limit.Value > 0 && limit.Unit != tokens.TokensUnit && limit.Unit != tokens.CharsUnit) {
		return Output{}, fmt.Errorf("engine: invalid input limit")
	}
	points := append([]Point(nil), input.Points...)
	candidates := make([]candidate, 0)
	for _, point := range points {
		if point.Name == "" {
			return Output{}, fmt.Errorf("engine: evaluation point name is required")
		}
		for _, item := range e.entries[input.Harness][point.Name] {
			entry := item.entry
			if !matchesTarget(entry, point) || !matchesSource(entry, point.Name, input.Source) {
				continue
			}
			whenInput := mergeWhenInput(point.When, input)
			if !item.matcher.Match(whenInput) {
				continue
			}
			values := mergeValues(input.Variables, point.Values, whenInput)
			modelText := entry.RenderModelText(values)
			userText := ""
			if entry.Action == "warn" {
				userText = entry.Body.Render(values)
			}
			candidates = append(candidates, candidate{entry: entry, point: point, modelText: modelText, userText: userText})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if lessOrder(left.entry.Order, right.entry.Order) {
			return true
		}
		if lessOrder(right.entry.Order, left.entry.Order) {
			return false
		}
		if left.entry.Entry != right.entry.Entry {
			return left.entry.Entry < right.entry.Entry
		}
		return left.point.Name < right.point.Name
	})

	blocks := make([]candidate, 0)
	for _, item := range candidates {
		if item.entry.Action != "block" {
			continue
		}
		if !item.point.Capabilities.Deny {
			return Output{}, fmt.Errorf("engine: block entry %q has no deny capability", item.entry.Entry)
		}
		blocks = append(blocks, item)
	}
	if len(blocks) > 0 {
		output := Output{Block: true}
		reasons := make([]string, 0, len(blocks))
		for _, item := range blocks {
			if strings.TrimSpace(item.modelText) == "" {
				return Output{}, fmt.Errorf("engine: block entry %q rendered an empty reason", item.entry.Entry)
			}
			reasons = append(reasons, item.modelText)
			output.Records = append(output.Records, Record{Entry: item.entry.Entry, Action: item.entry.Action, Result: "blocked"})
			// Claude's suppressOriginalPrompt field is valid only for the
			// ordinary UserPromptSubmit hook. UserPromptExpansion already
			// represents the expanded slash-command prompt and its codec does
			// not accept the suppression field.
			if isPromptPoint(item.point.Name) && input.Harness == "claude-code" {
				output.SuppressOriginalPrompt = true
			}
		}
		output.Reason = strings.Join(reasons, "\n")
		if err := output.ValidateFor(input.Harness); err != nil {
			return Output{}, err
		}
		return output, nil
	}

	accepted := make([]candidate, 0, len(candidates))
	output := Output{}
	for _, item := range candidates {
		if item.entry.Action == "block" {
			continue
		}
		if item.entry.Action == "inject" && !item.point.Capabilities.AddContext {
			releaseCandidates(accepted)
			return Output{}, fmt.Errorf("engine: inject entry %q has no add-context capability", item.entry.Entry)
		}
		modelText, userText := item.modelText, item.userText
		if item.entry.Action == "warn" {
			if !item.point.Capabilities.AddContext && !item.point.Capabilities.SystemMessage {
				releaseCandidates(accepted)
				return Output{}, fmt.Errorf("engine: warn entry %q has no output capability", item.entry.Entry)
			}
			if !item.point.Capabilities.AddContext {
				modelText = ""
			}
			if !item.point.Capabilities.SystemMessage {
				userText = ""
			}
		}
		if modelText == "" && userText == "" {
			continue
		}
		reservation, allowed, err := e.store.Claim(state.ClaimInput{
			Scope: input.Scope, Repeat: item.entry.Repeat, Action: item.entry.Action,
			Event: item.point.Name, Rule: item.entry.Rule, TurnID: input.TurnID,
			Body: modelText, ReinjectOnCompact: item.entry.ReinjectOnCompact,
		})
		if err != nil {
			releaseCandidates(accepted)
			return Output{}, err
		}
		if !allowed {
			output.Records = append(output.Records, Record{Entry: item.entry.Entry, Action: item.entry.Action, Result: "dedup"})
			continue
		}
		item.modelText, item.userText, item.reservation = modelText, userText, reservation
		accepted = append(accepted, item)
	}
	accepted, dropped := capCandidates(accepted, limit)
	var releaseErr error
	for _, item := range dropped {
		if err := item.reservation.Release(); err != nil {
			if releaseErr == nil {
				releaseErr = fmt.Errorf("engine: release capped entry %q: %w", item.entry.Entry, err)
			}
		}
	}
	if releaseErr != nil {
		releaseCandidates(accepted)
		releaseCandidates(dropped)
		return Output{}, releaseErr
	}
	for left, right := 0, len(dropped)-1; left < right; left, right = left+1, right-1 {
		dropped[left], dropped[right] = dropped[right], dropped[left]
	}
	contexts, messages := make([]string, 0), make([]string, 0)
	for _, item := range accepted {
		if item.modelText != "" {
			contexts = append(contexts, item.modelText)
		}
		if item.userText != "" {
			messages = append(messages, item.userText)
		}
		if item.reservation != nil {
			output.Reservations = append(output.Reservations, item.reservation)
		}
		output.Records = append(output.Records, Record{Entry: item.entry.Entry, Action: item.entry.Action, Result: "output"})
	}
	for _, item := range dropped {
		output.Records = append(output.Records, Record{Entry: item.entry.Entry, Action: item.entry.Action, Result: "cap-drop"})
	}
	output.AdditionalContext = strings.Join(contexts, "\n\n")
	output.SystemMessage = strings.Join(messages, "\n\n")
	if err := output.ValidateFor(input.Harness); err != nil {
		releaseReservations(output.Reservations)
		return Output{}, err
	}
	return output, nil
}

// Validate is the runtime output-whitelist assertion.
func (output Output) Validate() error {
	if output.Block {
		if strings.TrimSpace(output.Reason) == "" || output.AdditionalContext != "" || output.SystemMessage != "" {
			return fmt.Errorf("engine: blocked output must contain only a non-empty reason")
		}
		return nil
	}
	if output.Reason != "" || output.SuppressOriginalPrompt {
		return fmt.Errorf("engine: non-block output contains a block-only field")
	}
	return nil
}

// ValidateFor checks the output channels and provider-specific prompt field.
func (output Output) ValidateFor(harness string) error {
	if err := output.Validate(); err != nil {
		return err
	}
	if output.SuppressOriginalPrompt && harness != "claude-code" {
		return fmt.Errorf("engine: suppressOriginalPrompt is Claude Code only")
	}
	return nil
}

func releaseCandidates(candidates []candidate) {
	for _, item := range candidates {
		if item.reservation != nil {
			_ = item.reservation.Release()
		}
	}
}

func releaseReservations(reservations []*state.Reservation) {
	for _, reservation := range reservations {
		_ = reservation.Release()
	}
}

func capCandidates(candidates []candidate, limit tokens.InputLimit) ([]candidate, []candidate) {
	if limit.Value <= 0 || len(candidates) == 0 {
		return candidates, nil
	}
	kept := append([]candidate(nil), candidates...)
	dropped := make([]candidate, 0)
	for measure(joinModelText(kept), limit.Unit) > limit.Value && len(kept) > 0 {
		last := len(kept) - 1
		dropped = append(dropped, kept[last])
		kept = kept[:last]
	}
	return kept, dropped
}

func joinModelText(candidates []candidate) string {
	parts := make([]string, 0, len(candidates))
	for _, item := range candidates {
		if item.modelText != "" {
			parts = append(parts, item.modelText)
		}
	}
	return strings.Join(parts, "\n\n")
}

func measure(text string, unit tokens.Unit) int {
	estimate := tokens.EstimateText(text)
	if unit == tokens.CharsUnit {
		return estimate.Chars
	}
	return estimate.Tokens
}

func lessOrder(a, b compile.Order) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if a.PackageSeq != b.PackageSeq {
		return a.PackageSeq < b.PackageSeq
	}
	return a.RuleIndex < b.RuleIndex
}
