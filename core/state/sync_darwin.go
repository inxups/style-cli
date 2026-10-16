package state

import (
	"os"
	"syscall"
)

// syncFile uses fsync(2). os.File.Sync on Darwin issues F_FULLFSYNC, which
// also flushes the drive cache and costs several milliseconds per call; state
// files are small, expire after a week, and only need ordinary write-back.
func syncFile(file *os.File) error {
	return syscall.Fsync(int(file.Fd()))
}
