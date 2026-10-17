//go:build !windows

package install

// Used only by cross-platform renderer fixtures; never executed on POSIX.
func windowsLauncherPath() (string, error) {
	return "C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe", nil
}
