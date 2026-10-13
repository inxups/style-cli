package tokenestimate

import "testing"

func TestEstimate(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"abcd", 1},
		{"你好", 2},
		{"!", 1},
		{"a!", 1},
		{"abcde", 2},
	} {
		if got := Estimate(tc.text); got != tc.want {
			t.Errorf("Estimate(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}
