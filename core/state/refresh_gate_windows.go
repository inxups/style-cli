package state

import (
	"context"
	"errors"
	"fmt"
	"github.com/inxups/style-cli/internal/saferead"
	"os"
)

// Windows cannot rename a directory while its child lock handle is open.
// Every active refresh operation holds this permanent gate shared. Cleanup
// owns it exclusively while closing child locks and quarantining directories.
func platformRefreshGate(ctx context.Context, root *os.Root, exclusive, create bool) (*os.File, os.FileInfo, error) {
	flags := os.O_RDONLY
	if create {
		flags = os.O_RDWR
	}
	file, err := root.OpenFile(refreshGateName, flags, 0600)
	if create && errors.Is(err, os.ErrNotExist) {
		file, err = root.OpenFile(refreshGateName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			file, err = root.OpenFile(refreshGateName, os.O_RDWR, 0600)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.File, os.FileInfo, error) {
		unlockRefresh(file)
		file.Close()
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		return fail(fmt.Errorf("state: invalid refresh gate"))
	}
	if err = lockRefreshFile(ctx, file, !exclusive); err != nil {
		return fail(err)
	}
	named, err := root.Lstat(refreshGateName)
	if err != nil || !named.Mode().IsRegular() || !os.SameFile(named, info) {
		return fail(fmt.Errorf("state: refresh gate changed"))
	}
	return file, info, nil
}
func quarantineRefreshSession(g *RefreshGuard, name string) error {
	// The caller owns the exclusive platform gate, so no new refresh guard can
	// acquire this directory between closing the child handle and renaming it.
	if err := unlockRefresh(g.file); err != nil {
		return err
	}
	err := g.file.Close()
	g.file = nil
	if err != nil {
		return err
	}
	return g.root.Rename(g.session, name)
}

func platformCleanupGate(path string) (*os.File, error) {
	root, err := saferead.AnchorMutable(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, _, err := platformRefreshGate(context.Background(), root, true, true)
	return file, err
}
