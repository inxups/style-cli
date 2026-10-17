//go:build !windows

package state

import (
	"context"
	"os"
)

func platformRefreshGate(context.Context, *os.Root, bool, bool) (*os.File, os.FileInfo, error) {
	return nil, nil, nil
}
func quarantineRefreshSession(g *RefreshGuard, name string) error {
	return g.root.Rename(g.session, name)
}

func platformCleanupGate(string) (*os.File, error) { return nil, nil }
