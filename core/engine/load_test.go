package engine

import (
	"bytes"
	"os"
	"testing"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
)

func TestIndexConstructorsKeepValidation(t *testing.T) {
	for _, failure := range []string{"schema", "condition", "reevaluation", "nil-store"} {
		t.Run(failure, func(t *testing.T) {
			entry := testEntry("pkg/rule", "tool.pre", "event", "inject", "body", 1)
			index := testIndex(map[string][]compile.Entry{"tool.pre": {entry}})
			store := newTestStore(t)
			switch failure {
			case "schema":
				index.StyleVersion = ""
			case "condition":
				entry.When.Conditions = []when.Condition{{Field: "tool.input.command", Op: "regex", Pattern: "["}}
				index.Harnesses["claude-code"]["tool.pre"][0] = entry
			case "reevaluation":
				entry.Reevaluate = true
				index.Harnesses["claude-code"]["tool.pre"][0] = entry
			case "nil-store":
				store = nil
			}
			data, err := compile.Encode(index)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Load(bytes.NewReader(data), store); err == nil {
				t.Fatal("Load accepted invalid index/store")
			}
			if _, err := New(index, store); err == nil {
				t.Fatal("New accepted invalid index/store")
			}
		})
	}
}

func BenchmarkLoadIndex(b *testing.B) {
	data, err := os.ReadFile("../../testdata/golden/index.json")
	if err != nil {
		b.Fatal(err)
	}
	store, err := state.New(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(bytes.NewReader(data), store); err != nil {
			b.Fatal(err)
		}
	}
}
