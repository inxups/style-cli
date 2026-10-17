package runtime

import (
	"crypto/sha256"
	"fmt"
	"runtime"
)

func fixtureStateComponent(value string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("!%x", sha256.Sum256([]byte(value)))
	}
	return value
}
