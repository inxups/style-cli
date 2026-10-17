package engine

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/when"
)

func TestM3A1TraceParityAndClaimLifecycle(t *testing.T) {
	for _, mode := range []string{"output", "block", "cap", "empty", "missing", "target", "source"} {
		t.Run(mode, func(t *testing.T) {
			first := testEntry("pkg/first", "tool.pre", "event", "inject", "first", 1)
			first.When.ThinkingEffortApprox = nil
			second := testEntry("pkg/second", "tool.pre", "event", "warn", "Second. Detail.", 2)
			second.When.ThinkingEffortApprox = nil
			input := testInput("tool.pre")
			input.Points[0].Capabilities.SystemMessage = true
			limit := tokens.InputLimit{}
			switch mode {
			case "block":
				second.Action = "block"
				second.ModelBody = nil
			case "cap":
				limit = tokens.InputLimit{Value: 1, Unit: tokens.CharsUnit}
			case "empty":
				text := ""
				first.Body = compile.Body{Static: &text}
			case "missing":
				first.When.Conditions = []when.Condition{{Field: "tool.input.command", Op: "not_contains", Pattern: "secret"}}
			case "target":
				first.Target = "tool"
				first.Match = &compile.Match{Names: []string{"Unmatched"}}
			case "source":
				input.Points[0].Name = "session.start"
				input.Source = "resume"
				sources := []string{"startup"}
				first.Sources = &sources
				second.Sources = &sources
			}
			point := input.Points[0].Name
			first.Entry = first.Rule + "@" + point
			first.When.Event = point
			second.Entry = second.Rule + "@" + point
			second.When.Event = point
			normal := newTestEngine(t, map[string][]compile.Entry{point: {first, second}})
			diagnostic := newTestEngine(t, map[string][]compile.Entry{point: {first, second}})
			for iteration := 0; iteration < 2; iteration++ {
				want, err := normal.Process(input, limit)
				if err != nil {
					t.Fatal(err)
				}
				got, trace, err := diagnostic.ProcessWithTrace(input, limit)
				if err != nil {
					t.Fatal(err)
				}
				want.Reservations = nil
				got.Reservations = nil
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("output mismatch want=%+v got=%+v", want, got)
				}
				if len(trace.Points) != 1 || trace.Points[0].CandidateCount != 2 || len(trace.Entries) != 2 {
					t.Fatalf("trace=%+v", trace)
				}
				data, _ := json.Marshal(trace)
				if strings.Contains(string(data), "Second. Detail.") || strings.Contains(string(data), "secret") {
					t.Fatalf("trace leaks text: %s", data)
				}
				detail := trace.Entries[0]
				switch mode {
				case "output":
					if iteration == 0 && detail.Repeat != "claimed" || iteration == 1 && detail.Repeat != "repeat-seen" {
						t.Fatalf("claim ran twice: %+v", detail)
					}
				case "block":
					if detail.Block != "block-precedence" || detail.Repeat != "not-evaluated" {
						t.Fatalf("block claim %+v", detail)
					}
				case "cap":
					if detail.Output != "cap-drop" || !detail.ReservationReleased || detail.Repeat != "claimed" {
						t.Fatalf("cap failed release %+v", detail)
					}
				case "empty":
					if detail.Render != "empty-render" || detail.Repeat != "not-evaluated" {
						t.Fatalf("empty %+v", detail)
					}
				case "missing":
					if detail.When != "missing-data" || detail.Render != "not-evaluated" {
						t.Fatalf("missing %+v", detail)
					}
				case "target":
					if detail.Selector != "target-mismatch" || detail.When != "not-evaluated" {
						t.Fatalf("target %+v", detail)
					}
				case "source":
					if detail.Sources != "source-mismatch" {
						t.Fatalf("source %+v", detail)
					}
				}
			}
		})
	}
}

func TestM3A1TraceAcrossPointsUsesOneCapacityAndBlockDecision(t *testing.T) {
	for _, block := range []bool{false, true} {
		first := testEntry("pkg/pre", "tool.pre", "event", "inject", "a", 1)
		second := testEntry("pkg/post", "tool.post", "event", "warn", "Long warning.", 2)
		second.Repeat = "every"
		if block {
			first.Action = "block"
		}
		ordinary := newTestEngine(t, map[string][]compile.Entry{"tool.pre": {first}, "tool.post": {second}})
		diagnostic := newTestEngine(t, map[string][]compile.Entry{"tool.pre": {first}, "tool.post": {second}})
		input := testInput("tool.pre")
		post := testInput("tool.post").Points[0]
		post.Capabilities.SystemMessage = true
		input.Points = append(input.Points, post)
		limit := tokens.InputLimit{Value: 2, Unit: tokens.CharsUnit}
		for i := 0; i < 2; i++ {
			want, err := ordinary.Process(input, limit)
			if err != nil {
				t.Fatal(err)
			}
			got, trace, err := diagnostic.ProcessWithTrace(input, limit)
			if err != nil {
				t.Fatal(err)
			}
			want.Reservations = nil
			got.Reservations = nil
			if !reflect.DeepEqual(want, got) || len(trace.Points) != 2 || len(trace.Entries) != 2 {
				t.Fatalf("trace diverged %+v %+v %+v", want, got, trace)
			}
			if block {
				if !got.Block || trace.Entries[1].Block != "block-precedence" {
					t.Fatal("block did not cover all points")
				}
			} else {
				if trace.Entries[1].Output != "cap-drop" || trace.Entries[1].ReservationReleased || trace.Entries[1].Repeat != "repeat-bypass" {
					t.Fatal("capacity not shared or reservation retained")
				}
			}
		}
	}
}
