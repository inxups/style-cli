package probe

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/inxups/style-cli/internal/saferead"
	"github.com/speakeasy-api/agenthooks"
)

// Metadata is copied from a decoded event. It has no raw payload or mutable
// event pointers and is never marshalled; only its hashes enter a receipt.
type Metadata struct{ Harness, Event, Session, Agent, Turn string }
type Observer struct {
	home, harnessHome, harness, pluginRoot string
	pid                                    int
	now                                    func() time.Time
	clock                                  func() time.Time
	lookup                                 ProcessLookup
	once                                   sync.Once
	done                                   chan struct{}
}

// NewObserver performs only a fixed-position existence check when inactive;
// it creates no files or background work without an existing lease.
func NewObserver(home, harnessHome, harness, pluginRoot string) *Observer {
	if !safeHarness.MatchString(harness) {
		return nil
	}
	info, err := os.Lstat(filepath.Join(home, "probes", leaseName(harness)))
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxLeaseBytes {
		return nil
	}
	return &Observer{home: home, harnessHome: harnessHome, harness: harness, pluginRoot: pluginRoot, pid: os.Getpid(), now: time.Now, clock: time.Now, lookup: agenthooks.ReadProcessIdentity, done: make(chan struct{})}
}

// Submit returns immediately. A slow/failed reader cannot join the decision
// pipeline, and the task is never awaited during hook exit.
func (o *Observer) Submit(meta Metadata) {
	if o == nil || meta.Harness != o.harness || meta.Session == "" || meta.Event == "" {
		return
	}
	o.once.Do(func() { go func() { defer close(o.done); defer func() { _ = recover() }(); o.record(meta) }() })
}
func (o *Observer) record(meta Metadata) {
	deadline := o.clock().Add(ObserverBudget)
	expired := func() bool { return !o.clock().Before(deadline) }
	anchor, err := saferead.AnchorMutable(o.home)
	if err != nil {
		return
	}
	defer anchor.Close()
	if privateDirectory(anchor, "probes", false) != nil {
		return
	}
	root, err := anchor.OpenRoot("probes")
	if err != nil {
		return
	}
	defer root.Close()
	held, err := readLease(root, o.harness)
	if err != nil {
		return
	}
	lease := held.lease
	now := o.now().UTC()
	if now.Before(lease.Started) || !now.Before(lease.Expires) || !contains(lease.Binding.Events, meta.Event) {
		return
	}
	home, err := filepath.EvalSymlinks(o.home)
	if err != nil {
		return
	}
	harnessHome, err := filepath.EvalSymlinks(o.harnessHome)
	if err != nil {
		return
	}
	receipt := Receipt{Format: 1, Protocol: Protocol, RunID: lease.RunID, Harness: o.harness, HomeSHA256: HashString(home), HarnessHomeSHA256: HashString(harnessHome), Event: meta.Event, SessionSHA256: HashString(meta.Session), AgentSHA256: HashString(meta.Agent), TurnSHA256: HashString(meta.Turn), ObservedAt: now, Processes: []Process{}}
	if o.pluginRoot != "" {
		pluginRoot, err := filepath.EvalSymlinks(o.pluginRoot)
		if err != nil {
			return
		}
		receipt.PluginRootSHA256 = HashString(pluginRoot)
	}
	pid := o.pid
	for depth := 0; depth <= MaxAncestors && pid > 1; depth++ {
		if expired() {
			return
		}
		identity, err := o.lookup(pid)
		if err != nil {
			break
		}
		receipt.Processes = append(receipt.Processes, Process{identity.PID, identity.ParentPID, identity.Birth, identity.ExecutableID})
		if identity.ParentPID == pid {
			break
		}
		pid = identity.ParentPID
	}
	if expired() || !receipt.valid(lease) {
		return
	}
	if privateDirectory(root, lease.ReceiptDir, false) != nil {
		return
	}
	receipts, err := root.OpenRoot(lease.ReceiptDir)
	if err != nil {
		return
	}
	defer receipts.Close()
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > MaxReceiptBytes {
		return
	}
	current, err := readLease(root, o.harness)
	if err != nil || !bytes.Equal(current.data, held.data) || !os.SameFile(current.info, held.info) || !o.now().Before(lease.Expires) || expired() {
		return
	}
	name := ""
	for slot := 0; slot < MaxReceipts; slot++ {
		candidate := receiptFilename(slot)
		file, err := receipts.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return
		}
		name = candidate
		_, err = file.Write(data)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			receipts.Remove(name)
			return
		}
		break
	}
	if name == "" {
		return
	}
	current, err = readLease(root, o.harness)
	if err != nil || !bytes.Equal(current.data, held.data) || !os.SameFile(current.info, held.info) || !o.now().Before(lease.Expires) || expired() {
		receipts.Remove(name)
	}
}
func receiptFilename(slot int) string {
	return string([]byte{'0' + byte(slot/10), '0' + byte(slot%10)}) + ".json"
}
