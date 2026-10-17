package install

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
)

func windowsLauncherPath() (string, error) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.ToSlash(filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"))
	// The OS system directory is trusted; prohibit shell syntax instead of
	// guessing a dialect or expanding an attacker-controlled environment path.
	if !safeWindowsLauncher(path) {
		return "", fmt.Errorf("install: Windows system shell path cannot be represented safely")
	}
	if _, err := windows.UTF16PtrFromString(path); err != nil {
		return "", err
	}
	return path, nil
}
