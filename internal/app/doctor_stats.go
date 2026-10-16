package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/inxups/style-cli/core/state"
)

type RuleCost struct {
	Provider           string `json:"provider"`
	Rule               string `json:"rule"`
	EstimatedTokens    int64  `json:"estimated_tokens"`
	Emits              int    `json:"emits"`
	DistinctBodyHashes int    `json:"distinct_body_hashes"`
	RepeatedBody       bool   `json:"repeated_body"`
}

// Historical costs use the same bounded reader as stats. Corrupt records do
// not hide later valid records; the caller receives the aggregate and a warning.
func readRuleCosts(ctx context.Context, root string, now time.Time) ([]RuleCost, int, error) {
	type key struct{ provider, rule string }
	rules := map[key]*RuleCost{}
	hashes := map[key]map[string]bool{}
	legacy := 0
	cutoff := now.Add(-state.DefaultExpiry)
	quality, err := state.ScanStats(ctx, root, func(entry state.StatsEntry) error {
		if entry.Time.Before(cutoff) || entry.Time.After(now) || (entry.Result != "output" && entry.Result != "blocked") {
			return nil
		}
		if entry.Tokens == nil || entry.BodySHA256 == "" {
			if legacy == math.MaxInt {
				return fmt.Errorf("stats legacy count overflow")
			}
			legacy++
			return nil
		}
		k := key{entry.Provider, entry.Rule}
		if rules[k] == nil {
			rules[k] = &RuleCost{Provider: entry.Provider, Rule: entry.Rule}
			hashes[k] = map[string]bool{}
		}
		row := rules[k]
		if row.Emits == math.MaxInt || row.EstimatedTokens > math.MaxInt64-int64(*entry.Tokens) {
			return fmt.Errorf("stats aggregate exceeds int64")
		}
		row.Emits++
		row.EstimatedTokens += int64(*entry.Tokens)
		hashes[k][entry.BodySHA256] = true
		return nil
	})
	result := []RuleCost{}
	for k, row := range rules {
		row.DistinctBodyHashes = len(hashes[k])
		row.RepeatedBody = row.Emits >= 3 && row.DistinctBodyHashes == 1 && row.EstimatedTokens > 0
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.EstimatedTokens != right.EstimatedTokens {
			return left.EstimatedTokens > right.EstimatedTokens
		}
		return left.Provider < right.Provider || left.Provider == right.Provider && left.Rule < right.Rule
	})
	if len(result) > 3 {
		result = result[:3]
	}
	if err == nil && quality.Partial {
		err = fmt.Errorf("skipped stats metadata: invalid_lines=%d oversized_lines=%d skipped_symlinks=%d", quality.InvalidLines, quality.OversizedLines, quality.SkippedSymlinks)
	}
	return result, legacy, err
}
