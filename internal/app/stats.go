package app

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/inxups/style-cli/core/state"
)

type StatsRequest struct {
	Home  string
	By    string
	Since string
}

type StatsCounts struct {
	Outputs         int64 `json:"outputs"`
	Blocks          int64 `json:"blocks"`
	Dedup           int64 `json:"dedup"`
	CapDrops        int64 `json:"cap_drops"`
	EstimatedTokens int64 `json:"estimated_tokens"`
	Unestimated     int64 `json:"unestimated"`
}

type StatsRow struct {
	Provider string `json:"provider"`
	Rule     string `json:"rule,omitempty"`
	Session  string `json:"session,omitempty"`
	StatsCounts
}

type StatsWindow struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

type StatsResult struct {
	Format  int                `json:"format"`
	By      string             `json:"by"`
	Window  StatsWindow        `json:"window"`
	Totals  StatsCounts        `json:"totals"`
	Rows    []StatsRow         `json:"rows"`
	Quality state.StatsQuality `json:"quality"`
}

// Stats reports available historical runtime metadata without loading packages,
// detecting adapters, estimating bodies, or maintaining state.
func (s *Service) Stats(ctx context.Context, request StatsRequest) (StatsResult, error) {
	return statsAt(ctx, request, time.Now())
}

func statsAt(ctx context.Context, request StatsRequest, until time.Time) (StatsResult, error) {
	if err := checkContext(ctx); err != nil {
		return StatsResult{}, err
	}
	if request.By == "" {
		request.By = "rule"
	}
	if request.By != "rule" && request.By != "session" {
		return StatsResult{}, &UsageError{Message: "--by must be rule or session"}
	}
	window, err := statsWindow(request.Since, until)
	if err != nil {
		return StatsResult{}, err
	}
	home, err := resolveStyleHome(request.Home)
	if err != nil {
		return StatsResult{}, err
	}
	result := StatsResult{Format: 1, By: request.By, Window: window, Rows: []StatsRow{}}
	type group struct{ provider, id string }
	rows := map[group]*StatsRow{}
	result.Quality, err = state.ScanStats(ctx, filepath.Join(home, "state"), func(entry state.StatsEntry) error {
		if entry.Time.Before(window.Since) || entry.Time.After(window.Until) {
			return nil
		}
		key := group{entry.Provider, entry.Rule}
		if request.By == "session" {
			key.id = entry.Session
		}
		row := rows[key]
		if row == nil {
			row = &StatsRow{Provider: entry.Provider}
			if request.By == "session" {
				row.Session = key.id
			} else {
				row.Rule = key.id
			}
			rows[key] = row
		}
		counts := countsForRecord(entry.StatsRecord)
		if err := row.StatsCounts.add(counts); err != nil {
			return err
		}
		return result.Totals.add(counts)
	})
	if err != nil {
		return StatsResult{}, err
	}
	for _, row := range rows {
		result.Rows = append(result.Rows, *row)
	}
	sort.Slice(result.Rows, func(i, j int) bool {
		left, right := result.Rows[i], result.Rows[j]
		if left.EstimatedTokens != right.EstimatedTokens {
			return left.EstimatedTokens > right.EstimatedTokens
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		return left.Rule+left.Session < right.Rule+right.Session
	})
	if err := ctx.Err(); err != nil {
		return StatsResult{}, err
	}
	return result, nil
}

func statsWindow(since string, until time.Time) (StatsWindow, error) {
	window := StatsWindow{Until: until.UTC()}
	if since == "" {
		since = "168h"
	}
	if duration, err := time.ParseDuration(since); err == nil && duration > 0 {
		window.Since = window.Until.Add(-duration)
	} else {
		at, err := time.Parse(time.RFC3339Nano, since)
		if err != nil || at.After(window.Until) {
			return StatsWindow{}, &UsageError{Message: "--since requires a positive duration or RFC3339 timestamp no later than now"}
		}
		window.Since = at.UTC()
	}
	return window, nil
}

func countsForRecord(record state.StatsRecord) StatsCounts {
	var counts StatsCounts
	switch record.Result {
	case "output":
		counts.Outputs = 1
	case "blocked":
		counts.Blocks = 1
	case "dedup":
		counts.Dedup = 1
	case "cap-drop":
		counts.CapDrops = 1
	}
	if counts.Outputs+counts.Blocks != 0 {
		if record.Tokens == nil || record.BodySHA256 == "" {
			counts.Unestimated = 1
		} else {
			counts.EstimatedTokens = int64(*record.Tokens)
		}
	}
	return counts
}

func (counts *StatsCounts) add(other StatsCounts) error {
	for _, pair := range []struct {
		target *int64
		value  int64
	}{
		{&counts.Outputs, other.Outputs}, {&counts.Blocks, other.Blocks},
		{&counts.Dedup, other.Dedup}, {&counts.CapDrops, other.CapDrops},
		{&counts.EstimatedTokens, other.EstimatedTokens}, {&counts.Unestimated, other.Unestimated},
	} {
		if pair.value < 0 || *pair.target > math.MaxInt64-pair.value {
			return fmt.Errorf("stats aggregate exceeds int64")
		}
		*pair.target += pair.value
	}
	return nil
}
