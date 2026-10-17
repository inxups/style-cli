package compile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/schema"
)

func TestRefreshCompiledFormatAndPolicyEquivalence(t *testing.T) {
	if !state.RefreshSupported() {
		t.Skip("refresh platform")
	}
	for _, intervals := range [][]int{{0}, {3}, {3, 3}, {3, 5}, {0, 3}} {
		t.Run(fmt.Sprint(intervals), func(t *testing.T) {
			request := Request{StyleVersion: "0.3.0"}
			for i, n := range intervals {
				pkg := loadResolved(t, fmt.Sprintf("pkg-%d", i), fmt.Sprintf("  - id: r\n    target: event\n    event: prompt.submitted\n    text: periodic\n    refresh: %d\n    reason: periodic\n", n))
				request.Packages = append(request.Packages, inputFor(t, pkg, i+1))
			}
			result, err := Build(request)
			if err != nil {
				t.Fatal(err)
			}
			data, err := Encode(result.Index)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.ValidateIndex(data); err != nil {
				t.Fatal(err)
			}
			want := Format
			for _, n := range intervals {
				if n > 0 {
					want = RefreshFormat
				}
			}
			if result.Index.Format != want {
				t.Fatal(result.Index.Format)
			}
			fold := len(intervals) == 2 && intervals[0] == intervals[1]
			if (len(result.Deduplicated) > 0) != fold {
				t.Fatal(result.Deduplicated)
			}
			if want == Format && bytes.Contains(data, []byte(`"refresh"`)) {
				t.Fatal("zero changed wire")
			}
			provenance, err := BuildProvenance(request, result, data)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeProvenance(provenance, data, result.Index); err != nil {
				t.Fatal(err)
			}
			if want == RefreshFormat {
				old := bytes.Replace(data, []byte(`"format": 2`), []byte(`"format": 1`), 1)
				if schema.ValidateIndex(old) == nil {
					t.Fatal("format 1 accepted refresh")
				}
				var index Index
				if json.Unmarshal(old, &index) == nil {
					t.Fatal("strict decode accepted refresh in format 1")
				}
				for _, value := range []string{"0", "-1", "1.5", "1000001", "null", "true", "\"3\""} {
					bad := bytes.Replace(data, []byte(`"refresh": 3`), []byte(`"refresh": `+value), 1)
					if bytes.Equal(data, bad) {
						continue
					}
					if schema.ValidateIndex(bad) == nil {
						t.Fatalf("accepted %s", value)
					}
				}
			}
		})
	}
}
