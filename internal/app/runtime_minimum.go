package app

import "github.com/inxups/style-cli/core/compile"

// minimumIndex protects semantic additions that older management commands can
// otherwise accept as source and silently reinterpret during a rebuild.
func minimumIndex(format int) int {
	if format >= compile.UnknownFormat {
		return format
	}
	return 0
}
func validMinimumIndex(format int) bool { return format == compile.UnknownFormat }
