//go:build !darwin && !linux && !windows

package state

import (
	"fmt"
	"os"
)

func RefreshSupported() bool { return false }

const refreshOpenFlags = 0

func tryRefreshLock(*os.File, bool) (bool, error) {
	return false, fmt.Errorf("state: refresh-unsupported-platform")
}
func unlockRefresh(*os.File) error { return nil }
