package tokens

import (
	"fmt"
	"sort"

	"github.com/inxups/style-cli/core/plan"
)

// AnalyzeInstalled applies T003 to all package reports for one adapter. A
// block suppresses non-block output in its hook call, so the cost is the larger
// of the combined block reasons and combined inject/warn contents.
func AnalyzeInstalled(reports []Report, input InputLimit) ([]Diagnostic, error) {
	if input.Value <= 0 {
		return nil, fmt.Errorf("adapter input limit must be positive")
	}
	if input.Unit != TokensUnit && input.Unit != CharsUnit {
		return nil, fmt.Errorf("adapter input limit unit must be tokens or chars")
	}
	totals := make(map[string]*eventAccumulator)
	for _, report := range reports {
		for _, event := range report.Events {
			accumulator := totals[event.Key]
			if accumulator == nil {
				accumulator = &eventAccumulator{label: nonEmpty(event.Label, event.Key)}
				totals[event.Key] = accumulator
			} else if event.Label != "" && event.Label < accumulator.label {
				accumulator.label = event.Label
			}
			accumulator.block = add(accumulator.block, event.Block)
			accumulator.other = add(accumulator.other, event.Other)
			accumulator.hasBlock = accumulator.hasBlock || event.HasBlock
			accumulator.blockCount += event.BlockCount
			accumulator.otherCount += event.OtherCount
		}
	}
	findings := make([]Diagnostic, 0)
	for _, event := range sortedAccumulatorKeys(totals) {
		accumulator := totals[event]
		estimate := accumulator.worstCase()
		value := estimate.Tokens
		if input.Unit == CharsUnit {
			value = estimate.Chars
		}
		if float64(value) > float64(input.Value)*0.8 {
			findings = append(findings, Diagnostic{Code: "T003", Event: accumulator.label, Severity: "error", Message: fmt.Sprintf("installed estimate %d exceeds 80%% of adapter limit %d %s", value, input.Value, input.Unit)})
		}
	}
	sortDiagnostics(findings)
	return findings, nil
}

type placementEstimate struct {
	placement plan.Placement
	estimate  Estimate
}

type eventAccumulator struct {
	label      string
	block      Estimate
	other      Estimate
	hasBlock   bool
	blockCount int
	otherCount int
}

func (accumulator *eventAccumulator) add(action string, estimate Estimate) {
	if action == "block" {
		accumulator.block = add(accumulator.block, estimate)
		accumulator.hasBlock = true
		if estimate.Chars > 0 {
			accumulator.blockCount++
		}
		return
	}
	accumulator.other = add(accumulator.other, estimate)
	if estimate.Chars > 0 {
		accumulator.otherCount++
	}
}

func (accumulator *eventAccumulator) worstCase() Estimate {
	other := joinedEstimate(accumulator.other, accumulator.otherCount, false)
	if !accumulator.hasBlock {
		return other
	}
	block := joinedEstimate(accumulator.block, accumulator.blockCount, true)
	return Estimate{
		Tokens: max(block.Tokens, other.Tokens),
		Chars:  max(block.Chars, other.Chars),
	}
}

func joinedEstimate(estimate Estimate, count int, block bool) Estimate {
	if count <= 1 {
		return estimate
	}
	separatorChars := 2 // additionalContext uses one empty line between entries.
	if block {
		separatorChars = 1 // block reasons use a single newline.
	}
	return add(estimate, Estimate{Tokens: count - 1, Chars: (count - 1) * separatorChars})
}

func add(a, b Estimate) Estimate {
	return Estimate{Tokens: a.Tokens + b.Tokens, Chars: a.Chars + b.Chars}
}

func isAlwaysOn(placement plan.Placement, requirement plan.Requirement) bool {
	if placement.Event == "" && (placement.StaticSlot != "" || placement.StaticFallback) {
		return true
	}
	return placement.Event == plan.SessionStart && !requirement.HasWhen
}

func uniqueBuckets(values []Bucket) []Bucket {
	seen := make(map[string]struct{}, len(values))
	result := make([]Bucket, 0, len(values))
	for _, value := range values {
		if value.Key == "" {
			continue
		}
		if _, exists := seen[value.Key]; exists {
			continue
		}
		seen[value.Key] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func uniqueSorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	out := result[:0]
	for _, value := range result {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func sortedAccumulatorKeys(values map[string]*eventAccumulator) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
