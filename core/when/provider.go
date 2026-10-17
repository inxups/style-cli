package when

import "regexp"

var providerID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ValidProviderID accepts bounded configuration identifiers, never endpoints.
// unknown is reserved for the explicit missing-value selector.
func ValidProviderID(value string) bool {
	return value != "unknown" && providerID.MatchString(value)
}
