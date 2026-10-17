package compile

import (
	"bytes"
	"encoding/json"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/schema"
	"testing"
)

type a14NoDeny struct{ compileAdapter }

func (a a14NoDeny) Supports(event string, c plan.Capability) bool { return c != plan.Deny }

func TestM3A14FailFormatPlanningAndDedup(t *testing.T) {
	for _, action := range []string{"inject", "warn", "block"} {
		t.Run(action, func(t *testing.T) {
			pkg := loadResolved(t, "guard", "  - {id: r, target: event, event: tool.pre, action: "+action+", fail: closed, text: ok}\n  - {id: open, target: event, event: tool.pre, action: "+action+", text: ok}\n")
			result, err := Build(Request{StyleVersion: "x", Packages: []PackageInput{inputFor(t, pkg, 1)}})
			if err != nil || result.Index.Format != 7 || len(result.Index.Harnesses["codex"]["tool.pre"]) != 2 {
				t.Fatal(result, err)
			}
			data, _ := Encode(result.Index)
			if schema.ValidateIndex(data) != nil || json.Unmarshal(data, new(Index)) != nil {
				t.Fatal(string(data))
			}
			for _, n := range []byte{'1', '2', '3', '4', '5', '6'} {
				old := bytes.Replace(data, []byte(`"format": 7`), []byte(`"format": `+string(n)), 1)
				if schema.ValidateIndex(old) == nil || json.Unmarshal(old, new(Index)) == nil {
					t.Fatalf("old format %c accepted guard", n)
				}
			}
			p, err := plan.Build(pkg, a14NoDeny{compileAdapter{"codex"}})
			if err != nil || !p.Blocked || !p.Rules[0].Fatal || p.Rules[0].Fail != "closed" {
				t.Fatal(p, err)
			}
		})
	}
	pkg := loadResolved(t, "open", "  - {id: r, target: event, event: tool.pre, fail: open, text: ok}\n")
	r, err := Build(Request{StyleVersion: "x", Packages: []PackageInput{inputFor(t, pkg, 1)}})
	if err != nil || r.Index.Format != 1 {
		t.Fatal(r, err)
	}
}

func TestM3A14FailFallbackCannotWeaken(t *testing.T) {
	for _, fallback := range []string{"degrade", "skip", "error"} {
		t.Run(fallback, func(t *testing.T) {
			pkg := loadResolved(t, "guard", "  - {id: r, target: event, event: tool.pre, action: block, fail: closed, fallback: "+fallback+", text: ok}\n")
			p, err := plan.Build(pkg, a14NoDeny{compileAdapter{"codex"}})
			if err != nil || !p.Blocked || len(p.Rules[0].Placements) != 0 {
				t.Fatal(p, err)
			}
		})
	}
	// A selector excluding this adapter remains an intentional skip.
	pkg := loadResolved(t, "select", "  - {id: r, target: event, event: tool.pre, fail: closed, when: {harness: {name: claude-code}}, text: ok}\n")
	p, err := plan.Build(pkg, a14NoDeny{compileAdapter{"codex"}})
	if err != nil || p.Blocked || p.Rules[0].Status != plan.Skipped {
		t.Fatal(p, err)
	}
}
