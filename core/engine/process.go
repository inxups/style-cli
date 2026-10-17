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
	return e.process(input, limit, nil)
}

// ProcessWithTrace performs exactly one normal evaluation and records its safe decisions.
func (e *Engine) ProcessWithTrace(input Input, limit tokens.InputLimit) (Output, Trace, error) {
	trace := Trace{Points: make([]PointTrace, 0), Entries: make([]*EntryTrace, 0)}
	output, err := e.process(input, limit, &trace)
	sort.SliceStable(trace.Entries, func(i, j int) bool {
		a, b := trace.Entries[i], trace.Entries[j]
		if lessOrder(a.Order, b.Order) {
			return true
		}
		if lessOrder(b.Order, a.Order) {
			return false
		}
		if a.Entry != b.Entry {
			return a.Entry < b.Entry
		}
		return a.Point < b.Point
	})
	return output, trace, err
}

func (e *Engine) process(input Input, limit tokens.InputLimit, trace *Trace) (Output, error) {
	if e == nil || e.store == nil {
		return Output{}, fmt.Errorf("engine: uninitialized engine")
	}
	if input.Harness == "" || input.Scope.SessionID == "" || input.Scope.AgentID == "" {
		return Output{}, fmt.Errorf("engine: harness and session scope are required")
	}
	if input.Refresh != nil && !input.Refresh.Matches(input.Scope, input.Harness) {
		return Output{}, fmt.Errorf("engine: refresh frame does not match harness and scope")
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
		if trace != nil {
			trace.Points = append(trace.Points, PointTrace{point.Name, len(e.entries[input.Harness][point.Name])})
		}
		for _, item := range e.entries[input.Harness][point.Name] {
			entry := item.entry
			var detail *EntryTrace
			if trace != nil {
				detail = newEntryTrace(entry, point.Name)
				detail.Generation = input.Scope.Generation
				detail.Epoch = input.Scope.Epoch
				trace.Entries = append(trace.Entries, detail)
			}
			if !matchesTarget(entry, point) {
				if detail != nil {
					detail.Selector = "target-mismatch"
				}
				continue
			}
			if detail != nil {
				detail.Selector = "pass"
			}
			if !matchesSource(entry, point.Name, input.Source) {
				if detail != nil {
					detail.Sources = "source-mismatch"
				}
				continue
			}
			if detail != nil {
				detail.Sources = "pass"
			}
			whenInput := mergeWhenInput(point.When, input)
			matched := false
			if detail != nil {
				matched, detail.Conditions = item.matcher.MatchWithTrace(whenInput)
				detail.When = "pass"
				if !matched {
					detail.When = "predicate-false"
					for _, c := range detail.Conditions {
						if c.Result == "missing-data" {
							detail.When = "missing-data"
						}
					}
				}
			} else {
				matched = item.matcher.Match(whenInput)
			}
			if !matched {
				continue
			}
			values := mergeValues(input.Variables, point.Values, whenInput)
			populateEventValues(entry, values, whenInput)
			modelText := entry.RenderModelText(values)
			userText := ""
			if entry.Action == "warn" {
				userText = entry.Body.Render(values)
			}
			if detail != nil {
				detail.Render = "pass"
				detail.Model = renderSummary(modelText)
				detail.User = renderSummary(userText)
			}
			candidates = append(candidates, candidate{entry: entry, point: point, modelText: modelText, userText: userText, trace: detail})
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
		for _, item := range candidates {
			if item.trace != nil {
				item.trace.Block = "block-precedence"
				if item.entry.Action == "block" {
					item.trace.Block = "pass"
					item.trace.Repeat = "repeat-bypass"
				}
			}
		}
		output := Output{Block: true}
		reasons := make([]string, 0, len(blocks))
		for _, item := range blocks {
			if strings.TrimSpace(item.modelText) == "" {
				if item.trace != nil {
					item.trace.Render = "empty-render"
				}
				return Output{}, fmt.Errorf("engine: block entry %q rendered an empty reason", item.entry.Entry)
			}
			if item.trace != nil {
				item.trace.Block = "blocked"
				item.trace.Output = "blocked"
			}
			reasons = append(reasons, item.modelText)
			output.Records = append(output.Records, recordCandidate(item, "blocked"))
			// Claude's suppressOriginalPrompt field is valid only for the
			// ordinary UserPromptSubmit hook. UserPromptExpansion already
			// represents the expanded slash-command prompt and its codec does
			// not accept the suppression field.
			if isPromptPoint(item.point.Name) && item.point.Capabilities.SuppressOriginalPrompt {
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
		if item.trace != nil {
			item.trace.Block = "pass"
		}
		if modelText == "" && userText == "" {
			if item.trace != nil {
				item.trace.Render = "empty-render"
			}
			continue
		}
		claim := state.ClaimInput{
			Refresh: item.entry.Refresh, RefreshFrame: input.Refresh,
			Scope: input.Scope, Repeat: item.entry.Repeat, Action: item.entry.Action,
			Event: item.point.Name, Rule: item.entry.Rule, TurnID: input.TurnID,
			Body: modelText, ReinjectOnCompact: item.entry.ReinjectOnCompact,
		}
		if item.entry.Refresh > 0 {
			claim.RefreshTrace = &state.RefreshTrace{}
			if item.trace != nil {
				item.trace.Refresh = claim.RefreshTrace
			}
		}
		reservation, allowed, err := e.store.Claim(claim)
		if item.trace != nil {
			item.trace.Repeat = "claimed"
			item.trace.RepeatKeySHA256 = state.RepeatKeySHA256(claim)
			item.trace.Generation = input.Scope.Generation
			item.trace.Epoch = input.Scope.Epoch
			item.trace.TurnSource = input.TurnSource
			if item.trace.TurnSource == "" {
				item.trace.TurnSource = "unspecified"
			}
			if claim.Action == "block" || claim.Repeat == "every" || claim.Event == "subagent.start" {
				item.trace.Repeat = "repeat-bypass"
			}
		}
		if err != nil {
			releaseCandidates(accepted)
			return Output{}, err
		}
		if !allowed {
			if item.trace != nil {
				item.trace.Repeat = "repeat-seen"
			}
			output.Records = append(output.Records, recordCandidate(item, "dedup"))
			continue
		}
		item.modelText, item.userText, item.reservation = modelText, userText, reservation
		accepted = append(accepted, item)
	}
	accepted, dropped := capCandidates(accepted, limit)
	var releaseErr error
	for _, item := range dropped {
		if item.trace != nil {
			item.trace.Output = "cap-drop"
		}
		if err := item.reservation.Release(); err != nil {
			if releaseErr == nil {
				releaseErr = fmt.Errorf("engine: release capped entry %q: %w", item.entry.Entry, err)
			}
		} else if item.trace != nil {
			item.trace.ReservationReleased = item.reservation != nil
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
		if item.trace != nil {
			item.trace.Output = "output"
		}
		if item.modelText != "" {
			contexts = append(contexts, item.modelText)
		}
		if item.userText != "" {
			messages = append(messages, item.userText)
		}
		if item.reservation != nil {
			output.Reservations = append(output.Reservations, item.reservation)
		}
		output.Records = append(output.Records, recordCandidate(item, "output"))
	}
	for _, item := range dropped {
		output.Records = append(output.Records, recordCandidate(item, "cap-drop"))
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

// ValidateFor is the compatibility entry point for the provider-neutral whitelist.
// Adapters choose prompt capabilities; provider codecs validate native fields.
func (output Output) ValidateFor(harness string) error {
	if err := output.Validate(); err != nil {
		return err
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
