package adapter

import "testing"

func TestProjectVersionPrerequisite(t *testing.T) {
	for _, c := range []struct {
		actual string
		want   bool
	}{{"0.159.2", false}, {"0.159.3", true}, {"0.160.0", true}, {"1.0.0", true}, {"v0.159.3", true}, {"0.159.3 (Codex CLI)", true}, {"unknown", false}, {"0.159", false}, {"0.159.3-beta", false}, {"0.159.3.1", false}, {"-1.0.0", false}, {"", false}} {
		t.Run(c.actual, func(t *testing.T) {
			if got := VersionAtLeast(c.actual, "0.159.3"); got != c.want {
				t.Fatal(got, c.want)
			}
		})
	}
}
