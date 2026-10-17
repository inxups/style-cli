//go:build !darwin && !linux

package agenthooks

import "os"

func processBirth(int) (int, string, error) { return 0, "", ErrProcessUnsupported }
func processDoesNotExist(error) bool        { return false }
func processFileID(os.FileInfo) string      { return "" }
