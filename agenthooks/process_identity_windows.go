package agenthooks

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func processBirth(pid int) (int, string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, "", err
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, "", err
	}
	parent, err := procPPID(pid)
	return parent, fmt.Sprintf("%08x:%08x", creation.HighDateTime, creation.LowDateTime), err
}
func processDoesNotExist(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_FOUND) || errors.Is(err, windows.ERROR_NO_MORE_FILES)
}
func processFileID(path string, before os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(before, info) {
		return "", errors.New("executable identity changed")
	}
	var value windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x:%08x:%08x", value.VolumeSerialNumber, value.FileIndexHigh, value.FileIndexLow), nil
}
