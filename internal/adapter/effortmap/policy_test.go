package effortmap_test

import (
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter/claude"
	"github.com/inxups/style-cli/internal/adapter/codex"
	"github.com/inxups/style-cli/internal/adapter/effortmap"
)

func TestM3A13BuiltinRowsAndUserOverrides(t *testing.T) {
	for _, h := range []string{"codex", "claude-code"} {
		t.Run(h, func(t *testing.T) {
			table, err := codex.EffortTable()
			if h == "claude-code" {
				table, err = claude.EffortTable()
			}
			if err != nil {
				t.Fatal(err)
			}
			policy := effortmap.NewPolicy(h, compile.EffortMapping{h: {"high": "low", "future": "open"}})
			for _, raw := range append(append([]string{}, table.Exact...), "ultra", "persistent", "future", " HIGH ", "") {
				t.Run(raw, func(t *testing.T) {
					got := policy.Apply(state.EventUpdate{EffortRaw: raw}, state.Session{}, table)
					builtin := table.Resolve(raw)
					tier, source := builtin.Effort, "builtin"
					if tier == "" {
						source = "unmapped"
					}
					if raw == "high" || raw == " HIGH " {
						tier, source = "low", "user"
					}
					if raw == "future" {
						tier, source = "open", "user"
					}
					if got.ThinkingEffort != tier || got.EffortRaw != raw || got.EffortMappingSource != source || got.EffortMappingSHA256 == "" {
						t.Fatal(got, tier, source)
					}
					if source == "unmapped" && got.EffortApprox != nil || source == "user" && (got.EffortApprox == nil || !*got.EffortApprox) || source == "builtin" && (got.EffortApprox == nil || *got.EffortApprox) {
						t.Fatal(got)
					}
				})
			}
		})
	}
}

func TestM3A13PolicyRemapsCachedRawOnChangeAndRemoval(t *testing.T) {
	table, _ := codex.EffortTable()
	first := effortmap.NewPolicy("codex", compile.EffortMapping{"codex": {"future": "high"}})
	cached := state.Session{EffortRaw: "future", ThinkingEffort: "high", EffortResolved: true, EffortMappingSHA256: compile.EffortMapping{"codex": {"future": "high"}}.SHA256("codex"), EffortMappingSource: "user"}
	changed := effortmap.NewPolicy("codex", compile.EffortMapping{"codex": {"future": "low"}})
	if v := changed.Apply(state.EventUpdate{}, cached, table); v.ThinkingEffort != "low" || v.EffortMappingSHA256 == cached.EffortMappingSHA256 || !*v.EffortApprox {
		t.Fatal(v)
	}
	if v := effortmap.NewPolicy("codex", nil).Apply(state.EventUpdate{}, cached, table); v.ThinkingEffort != "" || v.EffortRaw != "future" || v.EffortApprox != nil || v.EffortMappingSHA256 != "" || v.EffortMappingSource != "" || !v.EffortMappingResolved {
		t.Fatal(v)
	}
	if v := first.Apply(state.EventUpdate{EffortRaw: "other"}, cached, table); v.ThinkingEffort != "" || v.EffortRaw != "other" || v.EffortApprox != nil {
		t.Fatal(v)
	}
	cached.EffortRaw = ""
	if v := first.Apply(state.EventUpdate{}, cached, table); v.ThinkingEffort != "" || v.EffortApprox != nil {
		t.Fatal("invented raw", v)
	}
}
