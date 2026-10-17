package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/inxups/style-cli/internal/saferead"
)

const DefaultExpiry = 7 * 24 * time.Hour

var safeComponentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Store owns the on-disk STYLE state directory. It is safe for concurrent use
// within one process. Cross-process repeat claims use exclusive file creation.
type Store struct {
	root string
	now  func() time.Time
	mu   sync.Mutex
}

// Session holds values shared by all agent scopes in one provider session.
type Session struct {
	Model          string    `json:"model,omitempty"`
	ThinkingEffort string    `json:"effort,omitempty"`
	EffortRaw      string    `json:"effort_raw,omitempty"`
	EffortApprox   *bool     `json:"effort_approx,omitempty"`
	EffortResolved bool      `json:"effort_resolved,omitempty"`
	HarnessVersion string    `json:"harness_version,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Scope stores counters for one agent within a provider session.
type Scope struct {
	SessionID   string    `json:"-"`
	AgentID     string    `json:"-"`
	Generation  uint64    `json:"gen"`
	Epoch       uint64    `json:"epoch"`
	PromptCount uint64    `json:"prompt_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EventUpdate contains normalized session and lifecycle facts.
type EventUpdate struct {
	Source          string
	PromptSubmitted bool
	Model           string
	ThinkingEffort  string
	EffortRaw       string
	EffortApprox    *bool
	EffortResolved  bool
	HarnessVersion  string
}

// New creates a state store rooted at root. The directory is created lazily.
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("state: root directory must not be empty")
	}
	return &Store{root: filepath.Clean(root), now: time.Now}, nil
}

// Update applies event state changes and returns current session and scope.
// PromptCount is incremented before returning, making it a suitable fallback
// turn identifier for a prompt.submitted event without a native turn ID.
func (s *Store) Update(sessionID, agentID string, update EventUpdate) (Session, Scope, error) {
	return s.update(sessionID, agentID, update, nil)
}

// UpdateLocked preserves the legacy schemas and counters while confining
// refresh-enabled writes to the guard's anchored state root.
func (s *Store) UpdateLocked(sessionID, agentID string, update EventUpdate, guard *RefreshGuard) (Session, Scope, error) {
	if guard == nil || guard.store != s || guard.session != component(sessionID) {
		return Session{}, Scope{}, fmt.Errorf("state: refresh guard mismatch")
	}
	return s.update(sessionID, agentID, update, guard)
}

func (s *Store) update(sessionID, agentID string, update EventUpdate, guard *RefreshGuard) (Session, Scope, error) {
	if s == nil {
		return Session{}, Scope{}, fmt.Errorf("state: nil store")
	}
	if sessionID == "" {
		return Session{}, Scope{}, fmt.Errorf("state: session ID must not be empty")
	}
	if agentID == "" {
		agentID = "main"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ensure := ensurePrivatePath
	read := readStateJSON
	write := writeJSON
	removeAll := os.RemoveAll
	touch := os.Chtimes
	if guard != nil {
		if err := guard.Check(); err != nil {
			return Session{}, Scope{}, err
		}
		rel := func(path string) string { name, _ := filepath.Rel(s.root, path); return name }
		ensure = func(_ string, parts ...string) error {
			for i := range parts {
				if err := guard.directory(filepath.Join(parts[:i+1]...)); err != nil {
					return err
				}
			}
			return nil
		}
		read = func(path string, target any) (bool, error) {
			data, err := saferead.File(guard.root, rel(path), 64<<10)
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, decodeState(data, target)
		}
		write = func(path string, value any) error { return guard.write(rel(path), value) }
		removeAll = func(path string) error {
			name := rel(path)
			if info, err := guard.root.Lstat(name); err == nil && !info.IsDir() {
				return fmt.Errorf("state: regular directory required")
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return guard.root.RemoveAll(name)
		}
		touch = func(path string, a, b time.Time) error { return guard.root.Chtimes(rel(path), a, b) }
	}

	now := s.now().UTC()
	sessionDir := filepath.Join(s.root, component(sessionID))
	if err := ensure(s.root, component(sessionID)); err != nil {
		return Session{}, Scope{}, fmt.Errorf("state: create session directory: %w", err)
	}
	agentDir := filepath.Join(sessionDir, component(agentID))
	if err := ensure(s.root, component(sessionID), component(agentID)); err != nil {
		return Session{}, Scope{}, fmt.Errorf("state: create agent directory: %w", err)
	}

	sessionPath, scopePath := filepath.Join(sessionDir, "session.json"), filepath.Join(agentDir, "scope.json")
	var session Session
	sessionExists, err := read(sessionPath, &session)
	if err != nil {
		return Session{}, Scope{}, fmt.Errorf("state: read session: %w", err)
	}
	var scope Scope
	scopeExists, err := read(scopePath, &scope)
	if err != nil {
		return Session{}, Scope{}, fmt.Errorf("state: read scope: %w", err)
	}
	scope.SessionID, scope.AgentID = sessionID, agentID
	sessionBefore, scopeBefore := cloneSession(session), scope

	switch update.Source {
	case "startup":
		// Startup starts a fresh scope even if a provider reuses its ID.
		scope.Generation, scope.Epoch, scope.PromptCount = 0, 0, 0
		if err := removeAll(filepath.Join(agentDir, "seen")); err != nil {
			return Session{}, Scope{}, fmt.Errorf("state: reset startup reservations: %w", err)
		}
	case "clear":
		if scope.Generation == ^uint64(0) {
			return Session{}, Scope{}, fmt.Errorf("state: generation counter overflow")
		}
		scope.Generation++
	case "compact":
		if scope.Epoch == ^uint64(0) {
			return Session{}, Scope{}, fmt.Errorf("state: epoch counter overflow")
		}
		scope.Epoch++
	}
	if update.PromptSubmitted {
		if scope.PromptCount == ^uint64(0) {
			return Session{}, Scope{}, fmt.Errorf("state: prompt counter overflow")
		}
		scope.PromptCount++
	}

	if update.Model != "" {
		session.Model = update.Model
	}
	if update.EffortRaw != "" {
		// A reported raw value is one complete effort reading. An unmapped
		// value has no tier, so it must clear the previous tier rather than
		// leave a stale one matching when.thinking_effort.
		session.EffortRaw = update.EffortRaw
		session.ThinkingEffort = update.ThinkingEffort
		session.EffortApprox = nil
		if update.EffortApprox != nil {
			value := *update.EffortApprox
			session.EffortApprox = &value
		}
	} else {
		if update.ThinkingEffort != "" {
			session.ThinkingEffort = update.ThinkingEffort
		}
		if update.EffortApprox != nil {
			value := *update.EffortApprox
			session.EffortApprox = &value
		}
	}
	if update.EffortResolved {
		session.EffortResolved = true
	}
	if update.HarnessVersion != "" {
		session.HarnessVersion = update.HarnessVersion
	}

	// Most hook events change nothing. Rewriting both files with fsync on every
	// event dominated hook latency, so an unchanged file is not rewritten;
	// UpdatedAt records the last change. The scope file's modification time is
	// still refreshed so Cleanup sees the session as active.
	if !sessionExists || !sameSession(session, sessionBefore) {
		session.UpdatedAt = now
		if err := write(sessionPath, session); err != nil {
			return Session{}, Scope{}, fmt.Errorf("state: write session: %w", err)
		}
	}
	if !scopeExists || !sameScope(scope, scopeBefore) {
		scope.UpdatedAt = now
		if err := write(scopePath, scope); err != nil {
			return Session{}, Scope{}, fmt.Errorf("state: write scope: %w", err)
		}
	} else if err := touch(scopePath, now, now); err != nil {
		return Session{}, Scope{}, fmt.Errorf("state: refresh scope time: %w", err)
	}
	if guard != nil {
		if err := guard.Check(); err != nil {
			return Session{}, Scope{}, err
		}
	}
	return session, scope, nil
}

func cloneSession(session Session) Session {
	if session.EffortApprox != nil {
		value := *session.EffortApprox
		session.EffortApprox = &value
	}
	return session
}

// sameSession and sameScope compare persisted fields other than UpdatedAt.
func sameSession(a, b Session) bool {
	if (a.EffortApprox == nil) != (b.EffortApprox == nil) || a.EffortApprox != nil && *a.EffortApprox != *b.EffortApprox {
		return false
	}
	return a.Model == b.Model && a.ThinkingEffort == b.ThinkingEffort && a.EffortRaw == b.EffortRaw &&
		a.EffortResolved == b.EffortResolved && a.HarnessVersion == b.HarnessVersion
}

func sameScope(a, b Scope) bool {
	return a.Generation == b.Generation && a.Epoch == b.Epoch && a.PromptCount == b.PromptCount
}

// Read returns state without refreshing its timestamps.
func (s *Store) Read(sessionID, agentID string) (Session, Scope, error) {
	if s == nil || sessionID == "" {
		return Session{}, Scope{}, fmt.Errorf("state: store and session ID are required")
	}
	if agentID == "" {
		agentID = "main"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessionDir := filepath.Join(s.root, component(sessionID))
	var session Session
	if err := readJSON(filepath.Join(sessionDir, "session.json"), &session); err != nil {
		return Session{}, Scope{}, err
	}
	var scope Scope
	if err := readJSON(filepath.Join(sessionDir, component(agentID), "scope.json"), &scope); err != nil {
		return Session{}, Scope{}, err
	}
	scope.SessionID, scope.AgentID = sessionID, agentID
	return session, scope, nil
}

// ReadSession returns the session-wide facts without requiring an agent scope
// to exist. Runtime adapters use it before enrichment to honor cached values.
func (s *Store) ReadSession(sessionID string) (Session, error) {
	if s == nil || sessionID == "" {
		return Session{}, fmt.Errorf("state: store and session ID are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var session Session
	if err := readJSON(filepath.Join(s.root, component(sessionID), "session.json"), &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// Cleanup removes session directories whose newest state file is older than
// maxAge. Call this from sync/doctor, not from a hook hot path.
func (s *Store) Cleanup(maxAge time.Duration) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("state: nil store")
	}
	if maxAge <= 0 {
		return 0, fmt.Errorf("state: cleanup age must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("state: scan state root: %w", err)
	}
	cutoff := s.now().Add(-maxAge)
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		latest, err := newestModTime(path)
		if err != nil {
			return removed, fmt.Errorf("state: inspect %q: %w", entry.Name(), err)
		}
		if latest.IsZero() || !latest.Before(cutoff) {
			continue
		}
		if RefreshSupported() {
			// Lock every expired candidate, including an old session that may
			// enable refresh concurrently. Quarantine so waiting old inode
			// holders fail their identity check, then close before unlinking.
			root, err := saferead.AnchorMutable(s.root)
			if err != nil {
				return removed, err
			}
			guard, err := openRefreshGuard(context.Background(), s, root, entry.Name(), false)
			if err != nil {
				if strings.Contains(err.Error(), "lock busy") {
					continue
				}
				return removed, err
			}
			latest, err := newestStateFileTime(path)
			if err != nil {
				guard.Close()
				return removed, err
			}
			if !latest.IsZero() && !latest.Before(cutoff) {
				guard.Close()
				continue
			}
			cleanupRoot, err := guard.root.OpenRoot(".")
			if err != nil {
				guard.Close()
				return removed, err
			}
			quarantine := ".expired-" + randomReservation()
			err = guard.Check()
			if err == nil {
				err = guard.root.Rename(entry.Name(), quarantine)
			}
			guard.Close()
			if err == nil {
				err = cleanupRoot.RemoveAll(quarantine)
			}
			cleanupRoot.Close()
			if err != nil {
				return removed, err
			}
			removed++
			continue
		}
		if !latest.IsZero() && latest.Before(cutoff) {
			if err := os.RemoveAll(path); err != nil {
				return removed, fmt.Errorf("state: remove expired session %q: %w", entry.Name(), err)
			}
			removed++
		}
	}
	return removed, nil
}

// Creating a cleanup guard changes the session directory and lock mtimes.
// Recheck the actual state files under the lock so a first refresh writer is
// observed without making a newly created guard keep an expired session alive.
func newestStateFileTime(root string) (time.Time, error) {
	var latest time.Time
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || path == filepath.Join(root, "refresh.lock") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest, err
}

func newestModTime(root string) (time.Time, error) {
	var latest time.Time
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest, err
}

func component(value string) string {
	if safeComponentPattern.MatchString(value) {
		return value
	}
	return "!" + sha256Sum(value)
}

func readJSON(path string, target any) error {
	_, err := readStateJSON(path, target)
	return err
}

// readStateJSON decodes a strict state file. A missing file leaves target
// unchanged and reports exists=false.
func readStateJSON(path string, target any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, decodeState(data, target)
}

func decodeState(data []byte, target any) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' || !utf8.Valid(data) {
		return fmt.Errorf("state: invalid JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".style-state-*.tmp")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := syncFile(file); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = syncFile(dir)
		_ = dir.Close()
	}
	return nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func ensurePrivatePath(root string, components ...string) error {
	if err := ensurePrivateDir(root); err != nil {
		return err
	}
	path := root
	for _, component := range components {
		path = filepath.Join(path, component)
		if err := ensurePrivateDir(path); err != nil {
			return err
		}
	}
	return nil
}
