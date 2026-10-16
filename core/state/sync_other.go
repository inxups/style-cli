//go:build !darwin

package state

import "os"

func syncFile(file *os.File) error {
	return file.Sync()
}
