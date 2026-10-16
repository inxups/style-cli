package state

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"
	"time"
)

// StatsQuality describes skipped input without retaining any raw log content.
type StatsQuality struct {
	Partial         bool  `json:"partial"`
	InvalidLines    int64 `json:"invalid_lines"`
	OversizedLines  int64 `json:"oversized_lines"`
	SkippedSymlinks int64 `json:"skipped_symlinks"`
}

// StatsEntry adds the historical directory key and validated timestamp. Session
// is a storage key, not necessarily the original harness session identifier.
type StatsEntry struct {
	StatsRecord
	Session string
	Time    time.Time
	Rule    string
}

// ScanStats reads only <session>/<agent>/stats.jsonl beneath root. It neither
// creates nor maintains state. Directory handles pin each level during a scan;
// symlinks are skipped before opening, and inode checks reject replaced paths.
func ScanStats(ctx context.Context, root string, visit func(StatsEntry) error) (StatsQuality, error) {
	var quality StatsQuality
	if ctx == nil || visit == nil {
		return quality, fmt.Errorf("state: stats context and visitor are required")
	}
	if err := ctx.Err(); err != nil {
		return quality, err
	}
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return quality, nil
	}
	if err != nil {
		return quality, statsReadError(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return quality, fmt.Errorf("state: stats root must be a directory, not a symlink")
	}
	dir, err := os.OpenRoot(root)
	if errors.Is(err, fs.ErrNotExist) {
		return quality, nil
	}
	if err != nil {
		return quality, statsReadError(err)
	}
	defer dir.Close()
	actual, err := dir.Stat(".")
	if err != nil {
		return quality, statsReadError(err)
	}
	if !os.SameFile(info, actual) {
		return quality, fmt.Errorf("state: stats root changed during scan")
	}
	err = scanStatsDir(ctx, dir, 0, "", &quality, visit)
	quality.Partial = quality.InvalidLines != 0 || quality.OversizedLines != 0 || quality.SkippedSymlinks != 0
	return quality, err
}

func scanStatsDir(ctx context.Context, dir *os.Root, depth int, session string, quality *StatsQuality, visit func(StatsEntry) error) error {
	listing, err := dir.Open(".")
	if err != nil {
		return statsReadError(err)
	}
	defer listing.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := listing.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := dir.Lstat(entry.Name())
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return statsReadError(err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if err := incrementStatsCount(&quality.SkippedSymlinks); err != nil {
					return err
				}
				continue
			}
			if depth < 2 && info.IsDir() {
				child, err := dir.OpenRoot(entry.Name())
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return statsReadError(err)
				}
				actual, err := child.Stat(".")
				if err == nil && !os.SameFile(info, actual) {
					err = fmt.Errorf("directory changed during scan")
				}
				key := session
				if depth == 0 {
					key = entry.Name()
				}
				if err == nil {
					err = scanStatsDir(ctx, child, depth+1, key, quality, visit)
				} else {
					err = statsReadError(err)
				}
				child.Close()
				if err != nil {
					return err
				}
			} else if depth == 2 && info.Mode().IsRegular() && entry.Name() == "stats.jsonl" {
				file, err := dir.Open(entry.Name())
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return statsReadError(err)
				}
				actual, err := file.Stat()
				if err == nil && !os.SameFile(info, actual) {
					err = fmt.Errorf("file changed during scan")
				}
				if err == nil {
					err = readStatsLines(ctx, file, session, quality, visit)
				} else {
					err = statsReadError(err)
				}
				file.Close()
				if err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			return ctx.Err()
		}
		if readErr != nil {
			return statsReadError(readErr)
		}
	}
}

// ReadSlice drains an oversized line in bounded fragments, then resumes at the
// next line. Scanner's token limit would otherwise discard the rest of a file.
func readStatsLines(ctx context.Context, input io.Reader, session string, quality *StatsQuality, visit func(StatsEntry) error) error {
	reader := bufio.NewReaderSize(input, 512)
	line := make([]byte, 0, 511)
	oversized := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		part, err := reader.ReadSlice('\n')
		if err != nil && err != bufio.ErrBufferFull && err != io.EOF {
			return statsReadError(err)
		}
		content := bytes.TrimSuffix(part, []byte{'\n'})
		if !oversized {
			if len(line)+len(content) >= 512 {
				oversized = true
				line = line[:0]
			} else {
				line = append(line, content...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if len(part) > 0 || len(line) > 0 || oversized {
			if oversized {
				if err := incrementStatsCount(&quality.OversizedLines); err != nil {
					return err
				}
			} else if entry, ok := decodeStatsLine(line, session); ok {
				if err := visit(entry); err != nil {
					return err
				}
			} else if err := incrementStatsCount(&quality.InvalidLines); err != nil {
				return err
			}
		}
		line, oversized = line[:0], false
		if err == io.EOF {
			return ctx.Err()
		}
	}
}

func decodeStatsLine(line []byte, session string) (StatsEntry, bool) {
	var record StatsRecord
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF {
		return StatsEntry{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, record.At)
	rule, point, hasPoint := strings.Cut(record.Entry, "@")
	pkg, id, hasID := strings.Cut(rule, "/")
	if err != nil || strings.TrimSpace(record.Provider) == "" || strings.TrimSpace(record.Event) == "" || strings.TrimSpace(record.Point) == "" ||
		!hasPoint || strings.TrimSpace(point) == "" || !hasID || strings.TrimSpace(pkg) == "" || strings.TrimSpace(id) == "" {
		return StatsEntry{}, false
	}
	switch record.Action {
	case "inject", "warn", "block":
	default:
		return StatsEntry{}, false
	}
	switch record.Result {
	case "output", "blocked", "dedup", "cap-drop":
	default:
		return StatsEntry{}, false
	}
	if record.Tokens != nil && *record.Tokens < 0 {
		return StatsEntry{}, false
	}
	if record.BodySHA256 != "" {
		digest, err := hex.DecodeString(strings.TrimPrefix(record.BodySHA256, "sha256:"))
		if err != nil || !strings.HasPrefix(record.BodySHA256, "sha256:") || len(digest) != 32 {
			return StatsEntry{}, false
		}
	}
	return StatsEntry{StatsRecord: record, Session: session, Time: at, Rule: rule}, true
}

func incrementStatsCount(value *int64) error {
	if *value == math.MaxInt64 {
		return fmt.Errorf("state: stats count overflow")
	}
	*value += 1
	return nil
}

func statsReadError(err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("state: cannot read stats: %w", fs.ErrPermission)
	}
	return fmt.Errorf("state: cannot read stats: I/O error")
}
