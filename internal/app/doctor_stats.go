package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

type ruleCosts struct {
	rules  map[string]*RuleCost
	hashes map[string]map[string]bool
	legacy int
	cutoff time.Time
	now    time.Time
}

// Only bounded metadata is decoded. Old records without estimates are counted
// separately; today's index must not be used to invent historical costs.
func readRuleCosts(ctx context.Context, root string, now time.Time) ([]RuleCost, int, error) {
	result := []RuleCost{}
	stats := ruleCosts{rules: map[string]*RuleCost{}, hashes: map[string]map[string]bool{}, cutoff: now.Add(-state.DefaultExpiry), now: now}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) && path == root {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Name() != "stats.jsonl" {
			return nil
		}
		return stats.read(ctx, path)
	})
	for key, rule := range stats.rules {
		rule.DistinctBodyHashes = len(stats.hashes[key])
		rule.RepeatedBody = rule.Emits >= 3 && rule.DistinctBodyHashes == 1 && rule.EstimatedTokens > 0
		result = append(result, *rule)
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
	return result, stats.legacy, err
}

func (stats *ruleCosts) read(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 512), 512)
	line := 0
	for scanner.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return err
		}
		var record state.StatsRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("invalid stats metadata at %s:%d", path, line)
		}
		at, err := time.Parse(time.RFC3339Nano, record.At)
		if err != nil {
			return fmt.Errorf("invalid stats timestamp at %s:%d", path, line)
		}
		if at.Before(stats.cutoff) || at.After(stats.now) || (record.Result != "output" && record.Result != "blocked") {
			continue
		}
		if record.Tokens == nil || record.BodySHA256 == "" {
			stats.legacy++
			continue
		}
		if *record.Tokens < 0 {
			return fmt.Errorf("invalid stats estimate at %s:%d", path, line)
		}
		rule, _, _ := strings.Cut(record.Entry, "@")
		key := record.Provider + "\x00" + rule
		if stats.rules[key] == nil {
			stats.rules[key] = &RuleCost{Provider: record.Provider, Rule: rule}
			stats.hashes[key] = map[string]bool{}
		}
		stats.rules[key].EstimatedTokens += int64(*record.Tokens)
		stats.rules[key].Emits++
		stats.hashes[key][record.BodySHA256] = true
	}
	return scanner.Err()
}
