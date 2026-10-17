//go:build !darwin && !linux && !windows

package agenthooks

import "os"

func processBirth(int) (int, string, error)             { return 0, "", ErrProcessUnsupported }
func processDoesNotExist(error) bool                    { return false }
func processFileID(string, os.FileInfo) (string, error) { return "", nil }
