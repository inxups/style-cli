package agenthooks

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func processBirth(pid int) (int, string, error) {
	value, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		// Darwin reports an absent PID as a successful zero-length sysctl. The
		// fixed-size helper converts that to EIO; distinguish it from real I/O errors.
		if errors.Is(err, unix.EIO) {
			raw, readErr := unix.SysctlRaw("kern.proc.pid", pid)
			if readErr == nil && len(raw) == 0 {
				return 0, "", ErrProcessGone
			}
		}
		return 0, "", err
	}
	if value == nil || int(value.Proc.P_pid) != pid {
		return 0, "", ErrProcessGone
	}
	return int(value.Eproc.Ppid), fmt.Sprintf("%d:%d", value.Proc.P_starttime.Sec, value.Proc.P_starttime.Usec), nil
}
func processDoesNotExist(err error) bool { return errors.Is(err, syscall.ESRCH) }
func processFileID(info os.FileInfo) string {
	if value, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", value.Dev, value.Ino)
	}
	return ""
}
