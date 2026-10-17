package when

import (
	"reflect"
	"testing"
)

type traceFields struct {
	calls  int
	values FieldMap
}

func (f *traceFields) ReadConditionField(key string) (string, bool) {
	f.calls++
	return f.values.ReadConditionField(key)
}
func TestM3A1MatchTraceShortCircuitAndMissing(t *testing.T) {
	for _, test := range []struct {
		name          string
		rule          Rule
		input         Input
		want          bool
		field, result string
		calls         int
	}{
		{"missing-model", Rule{Model: []string{"*"}, Conditions: []Condition{{"secret", "contains", "x"}}}, Input{}, false, "model", "missing-data", 0},
		{"provider", Rule{Provider: []string{"x"}}, Input{}, false, "provider", "missing-data", 0},
		{"missing-negative", Rule{Conditions: []Condition{{"missing", "not_contains", "x"}}}, Input{}, false, "conditions[0].missing", "missing-data", 1},
		{"false-short-circuit", Rule{Conditions: []Condition{{"value", "contains", "no"}, {"value", "contains", "yes"}}}, Input{}, false, "conditions[0].value", "predicate-false", 1},
		{"agent-default", Rule{}, Input{}, true, "agent", "pass", 0},
		{"harness-order", Rule{Harness: map[string][]string{"version": {"1"}, "name": {"codex"}}}, Input{Harness: map[string]string{"name": "wrong", "version": "1"}}, false, "harness.name", "predicate-false", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, err := Compile(test.rule)
			if err != nil {
				t.Fatal(err)
			}
			fields := &traceFields{values: FieldMap{"value": "yes"}}
			test.input.Fields = fields
			got, checks := m.MatchWithTrace(test.input)
			if got != test.want || fields.calls != test.calls {
				t.Fatalf("got %v, calls %d, checks %+v", got, fields.calls, checks)
			}
			last := checks[len(checks)-1]
			if last.Field != test.field || last.Result != test.result {
				t.Fatalf("checks=%+v", checks)
			}
			fields.calls = 0
			if m.Match(test.input) != got || fields.calls != test.calls {
				t.Fatal("trace changed evaluation")
			}
			_, again := m.MatchWithTrace(test.input)
			if !reflect.DeepEqual(checks, again) {
				t.Fatal("unstable trace")
			}
		})
	}
}
