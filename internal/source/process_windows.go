//go:build windows

package source

import "os/exec"

// Native Windows process-tree support is assessed separately by M3-D11.
func isolateProcess(cmd *exec.Cmd) {}
