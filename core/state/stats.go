package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AppendStats appends one small JSON record using a single O_APPEND write.
// Records are intentionally bounded and should contain metadata only.
func (s *Store) AppendStats(sessionID, agentID string, record []byte) error {
	if s == nil || sessionID == "" {
		return fmt.Errorf("state: store and session ID are required")
	}
	if agentID == "" {
		agentID = "main"
	}
	trimmed := bytes.TrimSpace(record)
	if len(trimmed) == 0 || len(trimmed) >= 512 || !json.Valid(trimmed) || bytes.ContainsAny(trimmed, "\r\n") {
		return fmt.Errorf("state: stats record must be valid single-line JSON under 512 bytes")
	}
	var fields struct {
		At       string `json:"at"`
		Provider string `json:"provider"`
		Event    string `json:"event"`
		Point    string `json:"point"`
		Entry    string `json:"entry"`
		Action   string `json:"action"`
		Result   string `json:"result"`
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fields); err != nil {
		return fmt.Errorf("state: invalid stats fields: %w", err)
	}
	if fields.At == "" || fields.Provider == "" || fields.Event == "" || fields.Point == "" || fields.Entry == "" || fields.Action == "" || fields.Result == "" {
		return fmt.Errorf("state: stats record is missing required metadata")
	}
	agentDir := filepath.Join(s.root, component(sessionID), component(agentID))
	if err := ensurePrivatePath(s.root, component(sessionID), component(agentID)); err != nil {
		return fmt.Errorf("state: create stats directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(agentDir, "stats.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("state: open stats log: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("state: restrict stats log permissions: %w", err)
	}
	line := append(append([]byte(nil), trimmed...), '\n')
	written, err := file.Write(line)
	if err != nil {
		return fmt.Errorf("state: append stats: %w", err)
	}
	if written != len(line) {
		return fmt.Errorf("state: append stats: short write")
	}
	return nil
}
