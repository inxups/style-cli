package state

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/inxups/style-cli/internal/saferead"
)

const RefreshLockWait = 20 * time.Millisecond

// RefreshGuard serializes the whole runtime operation, including shared
// session facts and claim rollback. The inode is permanent until cold cleanup;
// process death releases the OS lock without a stale-owner deletion protocol.
type RefreshGuard struct {
	store    *Store
	root     *os.Root
	file     *os.File
	rootInfo os.FileInfo
	fileInfo os.FileInfo
	session  string
	closed   bool
}

func (s *Store) BeginRefresh(ctx context.Context, sessionID string) (*RefreshGuard, error) {
	if s == nil || sessionID == "" {
		return nil, fmt.Errorf("state: refresh requires store and session")
	}
	if !RefreshSupported() {
		return nil, fmt.Errorf("state: refresh-unsupported-platform")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return nil, err
	}
	root, err := saferead.AnchorMutable(s.root)
	if err != nil {
		return nil, err
	}
	session := component(sessionID)
	if err := root.Mkdir(session, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		root.Close()
		return nil, err
	}
	return openRefreshGuard(ctx, s, root, session, false)
}

func openRefreshGuard(ctx context.Context, s *Store, root *os.Root, session string, shared bool) (*RefreshGuard, error) {
	fail := func(err error) (*RefreshGuard, error) { root.Close(); return nil, err }
	path := filepath.Join(session, "refresh.lock")
	if err := saferead.CheckDirectories(root, path); err != nil {
		return fail(err)
	}
	flags := os.O_RDONLY | refreshOpenFlags
	if !shared {
		flags = os.O_RDWR | refreshOpenFlags
	}
	f, err := root.OpenFile(path, flags, 0600)
	if !shared && errors.Is(err, os.ErrNotExist) {
		// Create exclusively, then open the winner's permanent inode. Besides
		// refusing symlinks, this avoids macOS concurrent O_CREATE returning
		// ENOENT in the synchronized process fixture. Active locks are never deleted.
		f, err = root.OpenFile(path, flags|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			f, err = root.OpenFile(path, flags, 0600)
		}
	}
	if err != nil {
		return fail(err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		f.Close()
		return fail(fmt.Errorf("state: invalid refresh lock"))
	}
	ri, err := root.Stat(".")
	if err != nil {
		f.Close()
		return fail(err)
	}
	g := &RefreshGuard{store: s, root: root, file: f, rootInfo: ri, fileInfo: info, session: session}
	deadline := time.Now().Add(RefreshLockWait)
	for {
		if err := ctx.Err(); err != nil {
			g.Close()
			return nil, err
		}
		acquired, err := tryRefreshLock(f, shared)
		if err != nil {
			g.Close()
			return nil, err
		}
		if acquired {
			if err := ctx.Err(); err != nil {
				g.Close()
				return nil, err
			}
			if err := g.Check(); err != nil {
				g.Close()
				return nil, err
			}
			return g, nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			g.Close()
			return nil, fmt.Errorf("state: refresh lock busy")
		}
		if left > time.Millisecond {
			left = time.Millisecond
		}
		timer := time.NewTimer(left)
		select {
		case <-ctx.Done():
			timer.Stop()
			g.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (g *RefreshGuard) Check() error {
	if g == nil || g.closed {
		return fmt.Errorf("state: refresh lock is closed")
	}
	info, err := os.Lstat(g.root.Name())
	if err != nil || !info.IsDir() || !os.SameFile(info, g.rootInfo) || info.Mode() != g.rootInfo.Mode() {
		return saferead.ErrChanged
	}
	path := filepath.Join(g.session, "refresh.lock")
	if err := saferead.CheckDirectories(g.root, path); err != nil {
		return err
	}
	info, err = g.root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, g.fileInfo) || info.Size() != 0 {
		return saferead.ErrChanged
	}
	return nil
}

func (g *RefreshGuard) Close() error {
	if g == nil || g.closed {
		return nil
	}
	g.closed = true
	err := unlockRefresh(g.file)
	if e := g.file.Close(); err == nil {
		err = e
	}
	if e := g.root.Close(); err == nil {
		err = e
	}
	return err
}

func (g *RefreshGuard) directory(name string) error {
	if err := g.Check(); err != nil {
		return err
	}
	// Root APIs confine all mutations; each directory is still checked to
	// reject aliases within that root, including an otherwise safe symlink.
	if err := g.root.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return saferead.CheckDirectories(g.root, filepath.Join(name, "placeholder"))
}

func (g *RefreshGuard) write(name string, value any) error {
	if err := g.Check(); err != nil {
		return err
	}
	if err := saferead.CheckDirectories(g.root, name); err != nil {
		return err
	}
	if info, err := g.root.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("state: regular file required")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data)+1 > maxRefreshBytes {
		return fmt.Errorf("state: refresh byte limit")
	}
	temp := filepath.Join(filepath.Dir(name), ".style-state-"+randomReservation()+".tmp")
	f, err := g.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|refreshOpenFlags, 0600)
	if err != nil {
		return err
	}
	defer g.root.Remove(temp)
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := syncFile(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := g.Check(); err != nil {
		return err
	}
	if err := g.root.Rename(temp, name); err != nil {
		return err
	}
	if dir, err := g.root.Open(filepath.Dir(name)); err == nil {
		_ = syncFile(dir)
		_ = dir.Close()
	}
	return g.Check()
}

func randomReservation() string {
	var value [16]byte
	// crypto/rand.Read either fills the buffer or terminates the process.
	_, _ = rand.Read(value[:])
	return fmt.Sprintf("%x", value)
}
