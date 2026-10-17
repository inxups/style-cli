package agenthooks

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func processBirth(pid int) (int, string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, "", err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return 0, "", errors.New("invalid process stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return 0, "", errors.New("short process stat")
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", err
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return 0, "", err
	}
	return parent, fields[19], nil
}
func processDoesNotExist(err error) bool { return errors.Is(err, syscall.ESRCH) }
func processFileID(info os.FileInfo) string {
	if value, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", value.Dev, value.Ino)
	}
	return ""
}
