package adapter

import (
	"fmt"
	"strings"
)

// VersionAtLeast rejects unknown versions before a version-bound route writes.
func VersionAtLeast(actual, minimum string) bool {
	parse := func(s string) ([3]int, bool) {
		var v [3]int
		fields := strings.Fields(strings.TrimPrefix(s, "v"))
		if len(fields) == 0 {
			return v, false
		}
		var tail string
		n, _ := fmt.Sscanf(fields[0], "%d.%d.%d%s", &v[0], &v[1], &v[2], &tail)
		return v, n == 3 && v[0] >= 0 && v[1] >= 0 && v[2] >= 0
	}
	a, ok := parse(actual)
	b, valid := parse(minimum)
	if !ok || !valid {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}
