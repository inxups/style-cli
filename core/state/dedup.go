package state

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Reservation is a successful exclusive claim on a repeat key.
type Reservation struct {
	release   func() error
	releaseMu sync.Mutex
	released  bool
	path      string
	once      sync.Once
	err       error
}

// ClaimInput describes the rule, rendered body, and event scope to reserve.
type ClaimInput struct {
	Refresh           int
	RefreshFrame      *RefreshFrame
	RefreshTrace      *RefreshTrace
	Scope             Scope
	Repeat            string
	Action            string
	Event             string
	Rule              string
	TurnID            string
	Body              string
	ReinjectOnCompact bool
}

// Claim returns allowed=false when a repeat reservation already exists.
// Blocks, repeat: every, and subagent.start entries are intentionally not
// deduplicated.
func (s *Store) Claim(input ClaimInput) (*Reservation, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("state: nil store")
	}
	if input.Refresh > 0 {
		return s.claimRefresh(input)
	}
	if input.Action == "block" || input.Repeat == "every" || input.Event == "subagent.start" {
		return nil, true, nil
	}
	if input.Scope.SessionID == "" || input.Scope.AgentID == "" || input.Rule == "" {
		return nil, false, fmt.Errorf("state: session, agent, and rule are required for repeat claim")
	}
	if input.RefreshFrame != nil {
		return s.claimOrdinaryLocked(input)
	}
	if input.Repeat != "session" && input.Repeat != "turn" {
		return nil, false, fmt.Errorf("state: unsupported repeat value %q", input.Repeat)
	}
	seenDir := filepath.Join(s.root, component(input.Scope.SessionID), component(input.Scope.AgentID), "seen")
	if err := ensurePrivatePath(s.root, component(input.Scope.SessionID), component(input.Scope.AgentID), "seen"); err != nil {
		return nil, false, fmt.Errorf("state: create seen directory: %w", err)
	}
	name := repeatName(input)
	path := filepath.Join(seenDir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("state: reserve repeat key: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, false, fmt.Errorf("state: close repeat reservation: %w", err)
	}
	return &Reservation{path: path}, true, nil
}

// Release removes a reservation, typically when the runtime drops output at
// the hard input cap. It is safe to call multiple times.
func (r *Reservation) Release() error {
	if r == nil {
		return nil
	}
	if r.release != nil {
		r.releaseMu.Lock()
		defer r.releaseMu.Unlock()
		if r.released {
			return nil
		}
		err := r.release()
		if err == nil {
			r.released = true
		}
		return err
	}
	r.once.Do(func() {
		if err := os.Remove(r.path); !errors.Is(err, os.ErrNotExist) {
			r.err = err
		}
	})
	return r.err
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func sha256Sum(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// RepeatKeySHA256 identifies the logical claim without exposing filesystem paths.
func RepeatKeySHA256(input ClaimInput) string {
	if input.Refresh > 0 {
		return "sha256:" + sha256Sum(refreshRepeatName(input))
	}
	if input.Action == "block" || input.Repeat == "every" || input.Event == "subagent.start" {
		return ""
	}
	return "sha256:" + sha256Sum(repeatName(input))
}

func repeatName(input ClaimInput) string {
	var name string
	if input.Repeat == "session" {
		epoch := uint64(0)
		if input.ReinjectOnCompact {
			epoch = input.Scope.Epoch
		}
		name = fmt.Sprintf("s.%d.%d.%s.%s", input.Scope.Generation, epoch, shortHash(input.Rule), shortHash(input.Body))
	} else {
		turn := input.TurnID
		if turn == "" {
			turn = strconv.FormatUint(input.Scope.PromptCount, 10)
		}
		name = fmt.Sprintf("t.%d.%s.%s.%s", input.Scope.Generation, shortHash(turn), shortHash(input.Rule), shortHash(input.Body))
	}
	return name
}
