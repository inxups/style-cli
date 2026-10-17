package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/inxups/style-cli/internal/saferead"
)

const (
	maxRefreshBytes = 4 << 20
	maxRefreshItems = 40000
)

var refreshHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var refreshNonce = regexp.MustCompile(`^[a-f0-9]{32}$`)

type refreshCursor struct {
	Last        uint64 `json:"last"`
	Reservation string `json:"reservation"`
}

func (c *refreshCursor) UnmarshalJSON(data []byte) error {
	var wire struct {
		Last        *uint64 `json:"last"`
		Reservation *string `json:"reservation"`
	}
	if err := decodeState(data, &wire); err != nil {
		return err
	}
	if wire.Last == nil || wire.Reservation == nil {
		return fmt.Errorf("state: incomplete refresh cursor")
	}
	c.Last = *wire.Last
	c.Reservation = *wire.Reservation
	return nil
}

// This namespace is intentionally independent of scope.json.prompt_count.
type refreshState struct {
	Format     int                      `json:"format"`
	Generation uint64                   `json:"gen"`
	Q          uint64                   `json:"q"`
	Turns      map[string]uint64        `json:"turns"`
	Cursors    map[string]refreshCursor `json:"cursors"`
}

// RefreshTrace contains only safe policy and counter facts.
type RefreshTrace struct {
	Interval int     `json:"interval"`
	Current  uint64  `json:"current"`
	Last     *uint64 `json:"last,omitempty"`
	Clock    string  `json:"clock"`
	Reason   string  `json:"reason"`
}

// RefreshFrame belongs to one event under an active session guard. Tools with
// a known old native ID retain that ID's original counter, even after prompts
// from later turns have been observed.
type RefreshFrame struct {
	guard     *RefreshGuard
	scope     Scope
	harness   string
	path      string
	state     refreshState
	q         uint64
	available bool
	quality   string
}

// Matches prevents a caller from evaluating another harness or scope under
// this event's clock. The guard must still be live when a claim is attempted.
func (f *RefreshFrame) Matches(scope Scope, harness string) bool {
	return f != nil && f.harness == harness && f.scope.SessionID == scope.SessionID &&
		f.scope.AgentID == scope.AgentID && f.scope.Generation == scope.Generation && f.scope.Epoch == scope.Epoch
}

func validateRefreshState(value refreshState) error {
	if value.Format != 1 || value.Turns == nil || value.Cursors == nil || len(value.Turns)+len(value.Cursors) > maxRefreshItems {
		return fmt.Errorf("state: invalid or excessive refresh state")
	}
	for key, q := range value.Turns {
		if !refreshHash.MatchString(key) || q == 0 || q > value.Q {
			return fmt.Errorf("state: invalid refresh receipt")
		}
	}
	for key, c := range value.Cursors {
		if !refreshHash.MatchString(key) || !refreshNonce.MatchString(c.Reservation) || c.Last > value.Q {
			return fmt.Errorf("state: invalid refresh cursor")
		}
	}
	return nil
}

func readRefresh(root *os.Root, path string) (refreshState, bool, error) {
	var value refreshState
	data, err := saferead.File(root, path, maxRefreshBytes)
	if errors.Is(err, os.ErrNotExist) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	value, err = decodeRefreshState(data)
	return value, true, err
}

func decodeRefreshState(data []byte) (refreshState, error) {
	var value refreshState
	if err := decodeState(data, &value); err != nil {
		return value, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return value, err
	}
	for _, key := range []string{"format", "gen", "q", "turns", "cursors"} {
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return value, fmt.Errorf("state: incomplete refresh state")
		}
	}
	return value, validateRefreshState(value)
}

// Preflight rejects corrupt clocks before lifecycle changes can remove old
// claims. Advance still captures the state under the same guard afterwards.
func (g *RefreshGuard) Preflight(agent, harness string) error {
	if agent == "" {
		agent = "main"
	}
	if harness == "" {
		return fmt.Errorf("state: refresh harness required")
	}
	if err := g.Check(); err != nil {
		return err
	}
	_, _, err := readRefresh(g.root, filepath.Join(g.session, component(agent), "refresh", component(harness)+".json"))
	return err
}

// ReadSessionLocked uses the same bounded, symlink-refusing capture as replay.
func (g *RefreshGuard) ReadSession() (Session, error) {
	var session Session
	if err := g.Check(); err != nil {
		return session, err
	}
	data, err := saferead.File(g.root, filepath.Join(g.session, "session.json"), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return session, nil
	}
	if err != nil {
		return session, err
	}
	return session, decodeState(data, &session)
}

func (g *RefreshGuard) Advance(scope Scope, harness string, update EventUpdate, nativeTurnID string) (*RefreshFrame, error) {
	if g == nil || scope.SessionID == "" || scope.AgentID == "" || harness == "" || component(scope.SessionID) != g.session {
		return nil, fmt.Errorf("state: invalid refresh frame")
	}
	if err := g.Check(); err != nil {
		return nil, err
	}
	dir := filepath.Join(g.session, component(scope.AgentID), "refresh")
	if err := g.directory(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, component(harness)+".json")
	value, exists, err := readRefresh(g.root, path)
	if err != nil {
		return nil, err
	}
	changed := false
	if !exists || update.Source == "startup" || value.Generation != scope.Generation {
		value = refreshState{Format: 1, Generation: scope.Generation, Turns: map[string]uint64{}, Cursors: map[string]refreshCursor{}}
		changed = true
	}
	f := &RefreshFrame{guard: g, scope: scope, harness: harness, path: path, state: value, q: value.Q, available: true, quality: "native-turn-id"}
	if nativeTurnID == "" {
		f.quality = "prompt-count-fallback"
	}
	key := sha256Sum(nativeTurnID)
	if update.PromptSubmitted {
		q, received := value.Turns[key]
		if nativeTurnID == "" || !received {
			if value.Q == ^uint64(0) {
				return nil, fmt.Errorf("state: refresh counter overflow")
			}
			value.Q++
			q = value.Q
			if nativeTurnID != "" {
				value.Turns[key] = q
			}
			changed = true
		}
		f.q = q
	} else if nativeTurnID != "" {
		if q, received := value.Turns[key]; received {
			f.q = q
		} else {
			f.available = false
			f.quality = "unregistered-turn-id"
		}
	}
	f.state = value
	if changed {
		if err := f.save(); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (f *RefreshFrame) save() error {
	if err := validateRefreshState(f.state); err != nil {
		return err
	}
	return f.guard.write(f.path, f.state)
}

func refreshKey(input ClaimInput) string {
	epoch := uint64(0)
	if input.ReinjectOnCompact {
		epoch = input.Scope.Epoch
	}
	return sha256Sum(fmt.Sprintf("refresh:1:%d:%d:%d:%s:%s", input.Scope.Generation, epoch, input.Refresh, input.Rule, sha256Sum(input.Body)))
}

func (s *Store) claimRefresh(input ClaimInput) (*Reservation, bool, error) {
	f := input.RefreshFrame
	if f == nil || f.guard == nil || f.guard.store != s || f.scope.SessionID != input.Scope.SessionID || f.scope.AgentID != input.Scope.AgentID || f.scope.Generation != input.Scope.Generation || f.scope.Epoch != input.Scope.Epoch || input.Refresh < 1 || input.Refresh > 1000000 || input.Repeat != "session" || (input.Action != "inject" && input.Action != "warn") || input.Event == "subagent.start" {
		return nil, false, fmt.Errorf("state: refresh requires a matching guarded frame and compatible policy")
	}
	if err := f.guard.Check(); err != nil {
		return nil, false, err
	}
	trace := input.RefreshTrace
	if trace == nil {
		trace = &RefreshTrace{}
	}
	*trace = RefreshTrace{Interval: input.Refresh, Current: f.q, Clock: f.quality, Reason: "initial"}
	key := refreshKey(input)
	previous, seen := f.state.Cursors[key]
	if seen {
		last := previous.Last
		trace.Last = &last
		if !f.available {
			trace.Reason = "clock-unavailable"
			return nil, false, nil
		}
		if f.q < previous.Last || f.q-previous.Last < uint64(input.Refresh) {
			trace.Reason = "not-due"
			return nil, false, nil
		}
		trace.Reason = "due"
	}
	seenDir := filepath.Join(f.guard.session, component(input.Scope.AgentID), "seen")
	if err := f.guard.directory(seenDir); err != nil {
		return nil, false, err
	}
	ordinary := filepath.Join(seenDir, repeatName(input))
	_, err := saferead.File(f.guard.root, ordinary, 0)
	oldOrdinary := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	if !seen && oldOrdinary {
		f.state.Cursors[key] = refreshCursor{Last: f.q, Reservation: randomReservation()}
		if err := f.save(); err != nil {
			delete(f.state.Cursors, key)
			return nil, false, err
		}
		last := f.q
		trace.Last = &last
		trace.Reason = "seeded"
		return nil, false, nil
	}
	var ordinaryInfo os.FileInfo
	if !oldOrdinary {
		file, err := f.guard.root.OpenFile(ordinary, os.O_CREATE|os.O_EXCL|os.O_WRONLY|refreshOpenFlags, 0600)
		if err != nil {
			return nil, false, err
		}
		ordinaryInfo, err = file.Stat()
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			_ = f.guard.root.Remove(ordinary)
			return nil, false, fmt.Errorf("state: create ordinary refresh claim")
		}
	}
	current := refreshCursor{Last: f.q, Reservation: randomReservation()}
	f.state.Cursors[key] = current
	if err := f.save(); err != nil {
		if seen {
			f.state.Cursors[key] = previous
		} else {
			delete(f.state.Cursors, key)
		}
		if !oldOrdinary {
			_ = f.guard.root.Remove(ordinary)
		}
		return nil, false, err
	}
	cursorRestored := false
	release := func() error {
		guard := f.guard
		if guard.closed {
			var err error
			guard, err = s.BeginRefresh(context.Background(), input.Scope.SessionID)
			if err != nil {
				return err
			}
			defer guard.Close()
		}
		value, exists, err := readRefresh(guard.root, f.path)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("state: refresh reservation disappeared")
		}
		if value.Generation != input.Scope.Generation {
			return nil
		}
		if value.Cursors[key].Reservation == current.Reservation {
			if seen {
				value.Cursors[key] = previous
			} else {
				delete(value.Cursors, key)
			}
			if err := guard.write(f.path, value); err != nil {
				return err
			}
			cursorRestored = true
		} else {
			now, exists := value.Cursors[key]
			if !cursorRestored || seen && (!exists || now != previous) || !seen && exists {
				return nil
			}
		}
		f.state = value
		if !oldOrdinary {
			info, err := guard.root.Lstat(ordinary)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err == nil && (!info.Mode().IsRegular() || !os.SameFile(info, ordinaryInfo)) {
				return saferead.ErrChanged
			}
			if err == nil {
				if err := guard.root.Remove(ordinary); err != nil {
					return err
				}
			}
		}
		trace.Reason = "cap-release"
		return nil
	}
	return &Reservation{release: release}, true, nil
}

func refreshRepeatName(input ClaimInput) string {
	return "r." + refreshKey(input) + "." + strconv.Itoa(input.Refresh)
}

func (s *Store) claimOrdinaryLocked(input ClaimInput) (*Reservation, bool, error) {
	f := input.RefreshFrame
	if f.guard.store != s || f.scope.SessionID != input.Scope.SessionID || f.scope.AgentID != input.Scope.AgentID || f.scope.Generation != input.Scope.Generation || f.scope.Epoch != input.Scope.Epoch || (input.Repeat != "session" && input.Repeat != "turn") {
		return nil, false, fmt.Errorf("state: ordinary guard mismatch")
	}
	g := f.guard
	dir := filepath.Join(g.session, component(input.Scope.AgentID), "seen")
	if err := g.directory(dir); err != nil {
		return nil, false, err
	}
	name := filepath.Join(dir, repeatName(input))
	file, err := g.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|refreshOpenFlags, 0600)
	if errors.Is(err, os.ErrExist) {
		if _, err := saferead.File(g.root, name, 0); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	info, err := file.Stat()
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = g.root.Remove(name)
		return nil, false, fmt.Errorf("state: create ordinary claim")
	}
	return &Reservation{release: func() error {
		guard := g
		if guard.closed {
			var err error
			guard, err = s.BeginRefresh(context.Background(), input.Scope.SessionID)
			if err != nil {
				return err
			}
			defer guard.Close()
		}
		current, err := guard.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !current.Mode().IsRegular() || !os.SameFile(info, current) {
			return saferead.ErrChanged
		}
		return guard.root.Remove(name)
	}}, true, nil
}
