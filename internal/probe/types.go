// Package probe owns short-lived callback observations, separate from state/stats.
package probe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/speakeasy-api/agenthooks"
)

// Protocol is also an embedded capability marker inspected without executing
// an installed runtime. A marker alone is never callback evidence.
const Protocol = "style-probe-observer/1/receipt-v1"
const MaxReceipts = 32
const MaxReceiptBytes = 4096
const MaxLeaseBytes = 16384
const ObserverBudget = 5 * time.Millisecond
const MaxAncestors = 6

var ErrBusy = errors.New("probe lease is busy or cannot be safely reclaimed")
var ErrChanged = errors.New("probe ownership changed")
var safeHarness = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var safeRun = regexp.MustCompile(`^[a-f0-9]{32}$`)
var safeHash = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type Binding struct {
	Harness           string   `json:"harness"`
	Route             string   `json:"route"`
	HomeSHA256        string   `json:"home_sha256"`
	HarnessHomeSHA256 string   `json:"harness_home_sha256"`
	RouteRootSHA256   string   `json:"route_root_sha256"`
	RuntimeID         string   `json:"runtime_id"`
	RuntimeSHA256     string   `json:"runtime_sha256"`
	HarnessID         string   `json:"harness_id"`
	HarnessSHA256     string   `json:"harness_sha256"`
	LockSHA256        string   `json:"lock_sha256"`
	ConfigSHA256      string   `json:"config_sha256"`
	Events            []string `json:"events"`
}

type Owner struct {
	PID   int    `json:"pid"`
	Birth string `json:"birth"`
}

type Lease struct {
	Format     int       `json:"format"`
	Protocol   string    `json:"protocol"`
	RunID      string    `json:"run_id"`
	Owner      Owner     `json:"owner"`
	Started    time.Time `json:"started"`
	Expires    time.Time `json:"expires"`
	ReceiptDir string    `json:"receipt_dir"`
	Binding    Binding   `json:"binding"`
}

type Process struct {
	PID          int    `json:"pid"`
	ParentPID    int    `json:"parent_pid"`
	Birth        string `json:"birth"`
	ExecutableID string `json:"executable_id"`
}

type Receipt struct {
	Format            int       `json:"format"`
	Protocol          string    `json:"protocol"`
	RunID             string    `json:"run_id"`
	Harness           string    `json:"harness"`
	HomeSHA256        string    `json:"home_sha256"`
	HarnessHomeSHA256 string    `json:"harness_home_sha256"`
	PluginRootSHA256  string    `json:"plugin_root_sha256,omitempty"`
	Event             string    `json:"event"`
	SessionSHA256     string    `json:"session_sha256"`
	AgentSHA256       string    `json:"agent_sha256,omitempty"`
	TurnSHA256        string    `json:"turn_sha256,omitempty"`
	ObservedAt        time.Time `json:"observed_at"`
	Processes         []Process `json:"processes"`
}

type ProcessLookup func(int) (agenthooks.ProcessIdentity, error)

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func HashString(value string) string {
	if value == "" {
		return ""
	}
	return Hash([]byte(value))
}

func decode(data []byte, value any) error {
	if !utf8.Valid(data) {
		return errors.New("UTF-8 JSON required")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
func validOwner(owner Owner) bool {
	return owner.PID > 1 && owner.Birth != "" && len(owner.Birth) <= 64 && !strings.ContainsAny(owner.Birth, "\x00\r\n")
}
func (lease Lease) valid() bool {
	b := lease.Binding
	if lease.Format != 1 || lease.Protocol != Protocol || !safeRun.MatchString(lease.RunID) || !validOwner(lease.Owner) || !safeHarness.MatchString(b.Harness) || b.Route == "" || len(b.Route) > 64 {
		return false
	}
	if lease.Started.IsZero() || !lease.Expires.After(lease.Started) || lease.Expires.Sub(lease.Started) > 10*time.Minute || lease.ReceiptDir != receiptDir(lease.RunID, b.Harness) {
		return false
	}
	for _, value := range []string{b.HomeSHA256, b.HarnessHomeSHA256, b.RouteRootSHA256, b.RuntimeID, b.RuntimeSHA256, b.HarnessID, b.HarnessSHA256, b.LockSHA256, b.ConfigSHA256} {
		if !safeHash.MatchString(value) {
			return false
		}
	}
	if len(b.Events) == 0 || len(b.Events) > 64 {
		return false
	}
	for _, event := range b.Events {
		if !recognizedEvent(b.Harness, event) {
			return false
		}
	}
	return true
}
func (receipt Receipt) valid(lease Lease) bool {
	if receipt.Format != 1 || receipt.Protocol != Protocol || receipt.RunID != lease.RunID || receipt.Harness != lease.Binding.Harness || receipt.HomeSHA256 != lease.Binding.HomeSHA256 || receipt.HarnessHomeSHA256 != lease.Binding.HarnessHomeSHA256 || receipt.ObservedAt.Before(lease.Started) || !receipt.ObservedAt.Before(lease.Expires) || !safeHash.MatchString(receipt.SessionSHA256) {
		return false
	}
	if !contains(lease.Binding.Events, receipt.Event) || !recognizedEvent(receipt.Harness, receipt.Event) {
		return false
	}
	for _, value := range []string{receipt.AgentSHA256, receipt.TurnSHA256, receipt.PluginRootSHA256} {
		if value != "" && !safeHash.MatchString(value) {
			return false
		}
	}
	if len(receipt.Processes) == 0 || len(receipt.Processes) > MaxAncestors+1 {
		return false
	}
	seen := map[int]bool{}
	for i, process := range receipt.Processes {
		if !validOwner(Owner{process.PID, process.Birth}) || process.ParentPID < 0 || !safeHash.MatchString(process.ExecutableID) || seen[process.PID] {
			return false
		}
		if i > 0 && receipt.Processes[i-1].ParentPID != process.PID {
			return false
		}
		seen[process.PID] = true
	}
	return true
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func leaseName(harness string) string         { return harness + ".lease.json" }
func receiptDir(runID, harness string) string { return "runs/" + runID + "/" + harness }

func recognizedEvent(harness, event string) bool {
	return agenthooks.RecognizesNativeEvent(agenthooks.Provider(harness), event) || harness == "claude-code" && event == "PostModelSwitch"
}
