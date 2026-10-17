package probe

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
)

type Options struct {
	Now      func() time.Time
	Lookup   ProcessLookup
	OwnerPID int
}
type heldLease struct {
	lease Lease
	data  []byte
	info  os.FileInfo
}
type Session struct {
	root        *os.Root
	home        string
	RunID       string
	owner       Owner
	leases      map[string]heldLease
	now         func() time.Time
	lookup      ProcessLookup
	runInfo     os.FileInfo
	markerReady bool
	stop        func(string, heldLease) error
}

type BeginError struct {
	Err     error
	Cleanup Cleanup
}

func (e *BeginError) Error() string { return e.Err.Error() }
func (e *BeginError) Unwrap() error { return e.Err }

type Cleanup struct {
	Complete  bool     `json:"complete"`
	Remaining []string `json:"remaining"`
}

// Begin acquires every requested lease, rolling back only this run on error.
func Begin(home string, bindings []Binding, timeout time.Duration, options Options) (result *Session, beginErr error) {
	if timeout < time.Second || timeout > 10*time.Minute || len(bindings) == 0 {
		return nil, errors.New("invalid probe targets or timeout")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Lookup == nil {
		options.Lookup = agenthooks.ReadProcessIdentity
	}
	if options.OwnerPID == 0 {
		options.OwnerPID = os.Getpid()
	}
	identity, err := options.Lookup(options.OwnerPID)
	if err != nil {
		return nil, err
	}
	owner := Owner{identity.PID, identity.Birth}
	if !validOwner(owner) || owner.PID != options.OwnerPID {
		return nil, errors.New("probe owner identity unavailable")
	}
	anchor, err := saferead.AnchorMutable(home)
	if err != nil {
		return nil, err
	}
	defer anchor.Close()
	if err = privateDirectory(anchor, "probes", true); err != nil {
		return nil, err
	}
	root, err := anchor.OpenRoot("probes")
	if err != nil {
		return nil, err
	}
	s := &Session{root: root, home: filepath.Join(home, "probes"), owner: owner, leases: map[string]heldLease{}, now: options.Now, lookup: options.Lookup}
	s.stop = s.stopLease
	failed := true
	defer func() {
		if failed {
			cleanup := s.Cleanup()
			root.Close()
			if !cleanup.Complete {
				beginErr = &BeginError{beginErr, cleanup}
			}
		}
	}()
	if err = privateDirectory(root, "runs", true); err != nil {
		return nil, err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return nil, err
	}
	s.RunID = hex.EncodeToString(token)
	runPath := "runs/" + s.RunID
	if err = root.Mkdir(runPath, 0o700); err != nil {
		return nil, err
	}
	s.runInfo, err = root.Lstat(runPath)
	if err != nil {
		return nil, err
	}
	marker, _ := json.Marshal(struct {
		RunID string `json:"run_id"`
		Owner Owner  `json:"owner"`
	}{s.RunID, owner})
	if _, err = writeNew(root, runPath+"/owner.json", marker); err != nil {
		return nil, err
	}
	s.markerReady = true
	started := s.now().UTC()
	for _, binding := range bindings {
		binding.Events = append([]string{}, binding.Events...)
		if _, exists := s.leases[binding.Harness]; exists {
			return nil, errors.New("duplicate probe target")
		}
		lease := Lease{1, Protocol, s.RunID, owner, started, started.Add(timeout), receiptDir(s.RunID, binding.Harness), binding}
		if !lease.valid() {
			return nil, errors.New("invalid probe binding")
		}
		if err = root.Mkdir(lease.ReceiptDir, 0o700); err != nil {
			return nil, err
		}
		if _, err = root.Lstat(leaseName(binding.Harness)); err == nil {
			if err = s.recover(binding.Harness); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		data, _ := json.Marshal(lease)
		info, writeErr := writeNew(root, leaseName(binding.Harness), data)
		if writeErr != nil {
			if errors.Is(writeErr, os.ErrExist) {
				return nil, ErrBusy
			}
			return nil, writeErr
		}
		s.leases[binding.Harness] = heldLease{lease, data, info}
	}
	failed = false
	return s, nil
}

func privateDirectory(root *os.Root, name string, create bool) error {
	if create {
		if err := root.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	if err := saferead.CheckDirectories(root, name); err != nil {
		return err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("private regular probe directory required")
	}
	return nil
}
func writeNew(root *os.Root, name string, data []byte) (os.FileInfo, error) {
	if err := saferead.CheckDirectories(root, name); err != nil {
		return nil, err
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if writeErr != nil {
		root.Remove(name)
		return nil, writeErr
	}
	if statErr != nil {
		root.Remove(name)
		return nil, statErr
	}
	if closeErr != nil {
		root.Remove(name)
		return nil, closeErr
	}
	return info, nil
}
func readLease(root *os.Root, harness string) (heldLease, error) {
	data, err := saferead.File(root, leaseName(harness), MaxLeaseBytes)
	if err != nil {
		return heldLease{}, err
	}
	var lease Lease
	if decode(data, &lease) != nil || !lease.valid() || lease.Binding.Harness != harness {
		return heldLease{}, ErrBusy
	}
	info, err := root.Lstat(leaseName(harness))
	if err != nil {
		return heldLease{}, err
	}
	if info.Mode().Perm() != 0o600 {
		return heldLease{}, ErrBusy
	}
	return heldLease{lease, data, info}, nil
}

func (s *Session) stopLease(harness string, want heldLease) error {
	actual, err := readLease(s.root, harness)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(actual.data, want.data) || !os.SameFile(actual.info, want.info) {
		return ErrChanged
	}
	stopped := ".stopped-" + want.lease.RunID + "-" + harness + ".json"
	if _, err = s.root.Lstat(stopped); !errors.Is(err, os.ErrNotExist) {
		return ErrChanged
	}
	if err = s.root.Rename(leaseName(harness), stopped); err != nil {
		return err
	}
	data, err := saferead.File(s.root, stopped, MaxLeaseBytes)
	info, statErr := s.root.Lstat(stopped)
	if err != nil || statErr != nil || !bytes.Equal(data, want.data) || !os.SameFile(info, want.info) {
		// A replacement must be restored without overwriting a concurrently acquired lease.
		if s.root.Link(stopped, leaseName(harness)) == nil {
			s.root.Remove(stopped)
		}
		return ErrChanged
	}
	return s.root.Remove(stopped)
}
func (s *Session) recover(harness string) error {
	old, err := readLease(s.root, harness)
	if err != nil {
		return ErrBusy
	}
	if !s.now().After(old.lease.Expires) {
		return ErrBusy
	}
	identity, err := s.lookup(old.lease.Owner.PID)
	if err == nil {
		if identity.PID != old.lease.Owner.PID || !validOwner(Owner{identity.PID, identity.Birth}) || identity.Birth == old.lease.Owner.Birth {
			return ErrBusy
		}
	}
	if err != nil && !errors.Is(err, agenthooks.ErrProcessGone) {
		return ErrBusy
	}
	if _, err = s.root.Lstat("runs/" + old.lease.RunID); err == nil {
		if _, err = s.verifyRun(old.lease.RunID, old.lease.Owner, nil); err != nil {
			return ErrBusy
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrBusy
	}
	if err = s.stopLease(harness, old); err != nil {
		return err
	}
	return s.removeRun(old.lease.RunID, old.lease.Owner, harness, nil)
}
func (s *Session) verifyRun(runID string, owner Owner, expected os.FileInfo) (os.FileInfo, error) {
	name := "runs/" + runID
	info, err := s.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || expected != nil && !os.SameFile(info, expected) {
		return nil, ErrChanged
	}
	data, err := saferead.File(s.root, name+"/owner.json", 1024)
	if err != nil {
		return nil, err
	}
	var marker struct {
		RunID string `json:"run_id"`
		Owner Owner  `json:"owner"`
	}
	if decode(data, &marker) != nil || marker.RunID != runID || marker.Owner != owner {
		return nil, ErrChanged
	}
	return info, nil
}
func (s *Session) removeRun(runID string, owner Owner, harness string, expected os.FileInfo) error {
	name := "runs/" + runID
	if _, err := s.root.Lstat(name); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := s.verifyRun(runID, owner, expected); err != nil {
		return err
	}
	if harness != "" {
		target := receiptDir(runID, harness)
		if err := privateDirectory(s.root, target, false); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := s.root.RemoveAll(target); err != nil {
			return err
		}
		dir, err := s.root.Open(name)
		if err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(2)
		dir.Close()
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if len(entries) > 1 || len(entries) == 1 && entries[0].Name() != "owner.json" {
			return nil
		}
	}
	return s.root.RemoveAll(name)
}

// Cleanup first deactivates leases, then removes the run; it never waits for an
// observer, which can only write into already existing receipt directories.
func (s *Session) Cleanup() Cleanup {
	result := Cleanup{Complete: true, Remaining: []string{}}
	if s == nil || s.root == nil {
		return result
	}
	names := make([]string, 0, len(s.leases))
	for name := range s.leases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := s.stop(name, s.leases[name]); err != nil {
			result.Complete = false
			result.Remaining = append(result.Remaining, filepath.Join(s.home, leaseName(name)))
		}
	}
	if s.RunID != "" && s.runInfo != nil {
		var err error
		if result.Complete {
			if s.markerReady {
				err = s.removeRun(s.RunID, s.owner, "", s.runInfo)
			} else {
				// Initialization failed before publishing any lease. The directory
				// inode is still required before removing this incomplete private run.
				info, statErr := s.root.Lstat("runs/" + s.RunID)
				if statErr != nil {
					err = statErr
				} else if !os.SameFile(info, s.runInfo) {
					err = ErrChanged
				} else {
					err = s.root.RemoveAll("runs/" + s.RunID)
				}
			}
		} else {
			err = ErrChanged
		}
		if err != nil {
			result.Complete = false
			result.Remaining = append(result.Remaining, filepath.Join(s.home, "runs", s.RunID))
		}
	}
	return result
}
func (s *Session) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}
func (s *Session) Lease(harness string) Lease {
	value := s.leases[harness].lease
	value.Binding.Events = append([]string{}, value.Binding.Events...)
	return value
}

// Receipts is bounded even in a directory containing hostile entries. Invalid
// records are quality diagnostics, never successful observations.
func (s *Session) Receipts(harness string) ([]Receipt, []string) {
	result := []Receipt{}
	quality := []string{}
	held, ok := s.leases[harness]
	if !ok {
		return result, []string{"probe-receipt-invalid"}
	}
	dir, err := s.root.Open(held.lease.ReceiptDir)
	if err != nil {
		return result, []string{"probe-receipt-unreadable"}
	}
	entries, err := dir.ReadDir(MaxReceipts + 1)
	dir.Close()
	if err != nil && err != io.EOF {
		return result, []string{"probe-receipt-unreadable"}
	}
	if len(entries) > MaxReceipts {
		return result, []string{"probe-receipt-limit"}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !regexpReceiptName(entry.Name()) {
			quality = append(quality, "probe-receipt-invalid")
			continue
		}
		name := held.lease.ReceiptDir + "/" + entry.Name()
		data, err := saferead.File(s.root, name, MaxReceiptBytes)
		if err != nil {
			quality = append(quality, "probe-receipt-invalid")
			continue
		}
		info, err := s.root.Lstat(name)
		if err != nil || info.Mode().Perm() != 0o600 {
			quality = append(quality, "probe-receipt-invalid")
			continue
		}
		var receipt Receipt
		if decode(data, &receipt) != nil || !receipt.valid(held.lease) {
			quality = append(quality, "probe-receipt-invalid")
			continue
		}
		result = append(result, receipt)
	}
	return result, quality
}
func regexpReceiptName(value string) bool {
	for i := 0; i < MaxReceipts; i++ {
		if value == fmt.Sprintf("%02d.json", i) {
			return true
		}
	}
	return false
}

// Active verifies the fixed lease still names this run, independently of expiry.
func (s *Session) Active(harness string) bool {
	want, ok := s.leases[harness]
	if !ok {
		return false
	}
	current, err := readLease(s.root, harness)
	return err == nil && bytes.Equal(current.data, want.data) && os.SameFile(current.info, want.info)
}
