package app

import (
	"testing"

	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/internal/adapter"
)

func TestProviderLayoutUsesNativeAggregateBuckets(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		rule      spec.Rule
		placement plan.Placement
		want      []string
	}{
		{
			name:      "Codex aggregates all MCP selectors",
			provider:  "codex",
			rule:      spec.Rule{Effective: map[string]any{"target": "mcp", "mcp": "github/create_pull_request"}},
			placement: plan.Placement{Event: plan.ToolPre},
			want:      []string{"PreToolUse mcp"},
		},
		{
			name:      "Claude aggregates all MCP selectors",
			provider:  "claude-code",
			rule:      spec.Rule{Effective: map[string]any{"target": "mcp", "mcp": "github/create_pull_request"}},
			placement: plan.Placement{Event: plan.ToolPre},
			want:      []string{"PreToolUse mcp"},
		},
		{
			name:      "Codex maps only observable canonical tools",
			provider:  "codex",
			rule:      spec.Rule{Effective: map[string]any{"target": "tool", "tool": []any{"file.edit", "file.write"}}},
			placement: plan.Placement{Event: plan.ToolPost},
			want:      []string{"PostToolUse apply_patch"},
		},
		{
			name:      "Claude maps canonical edit and write tools",
			provider:  "claude-code",
			rule:      spec.Rule{Effective: map[string]any{"target": "tool", "tool": []any{"file.edit", "file.write"}}},
			placement: plan.Placement{Event: plan.ToolPost},
			want: []string{"PostToolUse Edit", "PostToolUse MultiEdit", "PostToolUse NotebookEdit", "PostToolUse Write",
				"PostToolUseFailure Edit", "PostToolUseFailure MultiEdit", "PostToolUseFailure NotebookEdit", "PostToolUseFailure Write"},
		},
		{
			name:      "native and canonical selectors are additive",
			provider:  "claude-code",
			rule:      spec.Rule{Effective: map[string]any{"target": "tool", "tool": "file.edit", "names": "Edit"}},
			placement: plan.Placement{Event: plan.ToolPre},
			want:      []string{"PreToolUse Edit", "PreToolUse MultiEdit", "PreToolUse NotebookEdit"},
		},
		{
			name:      "Claude explicit skill buckets separate command names",
			provider:  "claude-code",
			rule:      spec.Rule{Effective: map[string]any{"target": "skill", "skill": []any{"review", "debug"}}},
			placement: plan.Placement{Event: plan.SkillExplicit},
			want:      []string{"UserPromptExpansion debug", "UserPromptExpansion review"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buckets := layoutFor(adapter.ProviderID(test.provider)).Buckets(test.rule, test.placement)
			got := make([]string, len(buckets))
			for i, bucket := range buckets {
				got[i] = bucket.Label
			}
			if len(got) != len(test.want) {
				t.Fatalf("bucket labels = %#v, want %#v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("bucket labels = %#v, want %#v", got, test.want)
				}
			}
		})
	}

	t.Run("distinct MCP selectors share a budget bucket", func(t *testing.T) {
		first := layoutFor("codex").Buckets(spec.Rule{Effective: map[string]any{"target": "mcp", "mcp": "github/create_pull_request"}}, plan.Placement{Event: plan.ToolPre})
		second := layoutFor("codex").Buckets(spec.Rule{Effective: map[string]any{"target": "mcp", "mcp": "calendar/create_event"}}, plan.Placement{Event: plan.ToolPre})
		if len(first) != 1 || len(second) != 1 || first[0].Key != second[0].Key {
			t.Fatalf("MCP buckets = %#v and %#v; want one shared native bucket", first, second)
		}
	})
}

func TestEstimateSessionCostRoundsUniformTriggerPositions(t *testing.T) {
	rule := spec.Rule{Effective: map[string]any{
		"id": "triggered", "target": "tool", "tool": "shell", "action": "inject", "repeat": "every",
	}, Body: "Review shell commands carefully."}
	pkg := &spec.ResolvedPackage{Package: &spec.Package{Metadata: map[string]any{}}, Rules: []spec.Rule{rule}}
	planned := plan.Report{Rules: []plan.RulePlan{{ID: "triggered", Status: plan.Native, Action: "inject", Target: "tool", Placements: []plan.Placement{{Event: plan.ToolPost, Action: "inject", Status: plan.Native}}}}}
	budget := tokens.Report{Rules: []tokens.RuleEstimate{{RuleID: "triggered", Estimate: tokens.Estimate{Tokens: 10}}}}
	got := estimateSessionCost(pkg, planned, budget, 10, 2)
	// round(10/3)=3 and round(20/3)=7, so the cache factors sum to 3.5.
	estimate, err := tokens.EstimatePlacement(rule, planned.Rules[0].Placements[0], pkg)
	if err != nil {
		t.Fatal(err)
	}
	want := float64(estimate.Tokens) * 3.5
	if got.Requests != 10 || got.Triggers != 2 || got.Triggered != want || got.Total != want {
		t.Fatalf("session cost = %#v, want %v triggered units", got, want)
	}
}
