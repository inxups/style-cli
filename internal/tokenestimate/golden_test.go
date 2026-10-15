package tokenestimate

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFixedTextMatchesGoldenCounts(t *testing.T) {
	data, err := os.ReadFile("testdata/counts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string `json:"name"`
		Text   string `json:"text"`
		Tokens int    `json:"tokens"`
		Chars  int    `json:"chars"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 9 {
		t.Fatalf("golden has %d cases, want 9", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := CountText(tc.Text); got.Tokens != tc.Tokens || got.Chars != tc.Chars {
				t.Fatalf("CountText(%q)=%+v, want tokens=%d chars=%d", tc.Text, got, tc.Tokens, tc.Chars)
			}
		})
	}
}
