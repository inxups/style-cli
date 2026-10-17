package agenthooks

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrProcessGone = errors.New("process no longer exists")
var ErrProcessUnsupported = errors.New("process identity is unsupported on this platform")

// ProcessIdentity contains OS facts, never argv or environment. Executable is
// for in-memory verification; consumers should persist only ExecutableID.
type ProcessIdentity struct {
	PID          int
	ParentPID    int
	Birth        string
	Executable   string
	ExecutableID string
}

// ReadProcessIdentity uses kernel facilities, without running another program.
func ReadProcessIdentity(pid int) (ProcessIdentity, error) {
	if pid <= 0 {
		return ProcessIdentity{}, ErrProcessGone
	}
	parent, birth, err := processBirth(pid)
	if err != nil {
		return ProcessIdentity{}, processIdentityError(err)
	}
	executable, err := procExecutable(pid)
	if err != nil {
		return ProcessIdentity{}, processIdentityError(err)
	}
	id, err := ExecutableIdentity(executable)
	if err != nil {
		return ProcessIdentity{}, processIdentityError(err)
	}
	parentAfter, birthAfter, err := processBirth(pid)
	if err != nil {
		return ProcessIdentity{}, processIdentityError(err)
	}
	if birth != birthAfter || parent != parentAfter {
		return ProcessIdentity{}, errors.New("process identity changed")
	}
	return ProcessIdentity{pid, parent, birth, executable, id}, nil
}

func processIdentityError(err error) error {
	if errors.Is(err, os.ErrNotExist) || processDoesNotExist(err) {
		return ErrProcessGone
	}
	return err
}

// ExecutableIdentity is a small, stable identity of the current file. It is
// deliberately not a binary hash; the controller must hash the bound file.
func ExecutableIdentity(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("absolute executable required")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("regular executable required")
	}
	fileID, err := processFileID(canonical, info)
	if err != nil {
		return "", err
	}
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d", canonical, fileID, info.Size(), info.Mode(), info.ModTime().UnixNano())
	digest := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
