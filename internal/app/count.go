package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/tokencount"
)

type PlacementCount struct {
	Rule  string            `json:"rule"`
	Point string            `json:"point"`
	Count tokencount.Result `json:"count"`
}

func (s *Service) countText(ctx context.Context, text string, o tokencount.Options) (tokencount.Result, error) {
	counter := tokencount.Counter{}
	if s.counter != nil {
		counter = *s.counter
	}
	return counter.Count(ctx, text, o)
}
func (s *Service) planCounts(ctx context.Context, result *PlanResult, pkg *spec.ResolvedPackage, o tokencount.Options) error {
	if !o.Exact() {
		return nil
	}
	input := compile.PackageInput{Package: pkg, Sequence: 1, Lock: "sha256:" + strings.Repeat("0", 64)}
	for _, report := range result.Reports {
		input.Plans = append(input.Plans, report.Plan)
	}
	for _, p := range input.Plans {
		if p.Blocked {
			return fmt.Errorf("count: cannot count a blocked plan")
		}
	}
	built, err := compile.Build(compile.Request{StyleVersion: "count-preview", Packages: []compile.PackageInput{input}})
	if err != nil {
		return fmt.Errorf("count: cannot compile planned text: %w", err)
	}
	type pending struct {
		report            int
		rule, point, text string
		none              bool
	}
	items := []pending{}
	for ri, report := range result.Reports {
		for _, static := range built.StaticBodies {
			if static.Harness == report.Provider {
				items = append(items, pending{ri, static.Rule, static.Slot, static.Body, false})
			}
		}
		events := built.Index.Harnesses[report.Provider]
		points := make([]string, 0, len(events))
		for point := range events {
			points = append(points, point)
		}
		sort.Strings(points)
		for _, point := range points {
			for _, entry := range events[point] {
				none := entry.Action == "block" && (point == plan.PromptSubmitted || point == plan.SkillExplicit)
				body := entry.Body
				if entry.ModelBody != nil {
					body = *entry.ModelBody
				}
				if !none && body.Static == nil {
					return fmt.Errorf("count: exact plan has runtime text; use explain with explicit event/facts")
				}
				text := ""
				if !none {
					text = entry.RenderModelText(nil)
				}
				items = append(items, pending{ri, entry.Rule, point, text, none || text == ""})
			}
		}
	}
	// Preflight every text before any remote call, so one later dynamic/oversize
	// placement cannot turn a rejected plan into a partial network disclosure.
	for _, item := range items {
		if len(item.text) > tokencount.MaxTextBytes {
			return fmt.Errorf("count: planned text exceeds 256 KiB")
		}
	}
	for _, item := range items {
		var counted tokencount.Result
		if item.none {
			counted, err = tokencount.None(o)
		} else {
			counted, err = s.countText(ctx, item.text, o)
		}
		if err != nil {
			return err
		}
		result.Reports[item.report].Counts = append(result.Reports[item.report].Counts, PlacementCount{item.rule, item.point, counted})
	}
	for i := range result.Reports {
		result.Reports[i].CountScope = "individual compiled model-text placements; excludes native output, host markers/envelopes and runtime matching; counts are not full-request totals"
	}
	return nil
}
func modelText(output engine.Output) string {
	if !output.Block {
		return output.AdditionalContext
	}
	for _, record := range output.Records {
		if record.Point == plan.ToolPre {
			return output.Reason
		}
	}
	return ""
}
