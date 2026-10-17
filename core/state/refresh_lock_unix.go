//go:build darwin || linux

package state

import (
	"errors"
	"os"
	"syscall"
)

func RefreshSupported() bool { return true }

const refreshOpenFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func tryRefreshLock(f *os.File, shared bool) (bool, error) {
	op := syscall.LOCK_EX
	if shared {
		op = syscall.LOCK_SH
	}
	err := syscall.Flock(int(f.Fd()), op|syscall.LOCK_NB)
	if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockRefresh(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
