package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func readerStatsLine(t *testing.T) []byte {
	t.Helper()
	tokens := 12
	data, err := json.Marshal(StatsRecord{At: "2026-10-16T00:00:00Z", Provider: "retired", Event: "PreToolUse", Point: "tool.pre", Entry: "removed/check@tool.pre", Action: "inject", Result: "output", Tokens: &tokens, BodySHA256: "sha256:" + strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func putReaderStats(t *testing.T, root, session, agent string, data []byte) string {
	t.Helper()
	path := filepath.Join(root, session, agent, "stats.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanStatsStrictMetadataContinuesAfterInvalidLines(t *testing.T) {
	valid := readerStatsLine(t)
	for _, test := range []struct{ field, value string }{
		{"at", `"invalid"`}, {"at", `""`}, {"provider", `""`}, {"provider", `"  "`},
		{"event", `""`}, {"point", `""`}, {"entry", `"pkg/id"`}, {"entry", `"pkg/id@"`},
		{"entry", `"id@tool.pre"`}, {"entry", `"/id@tool.pre"`}, {"entry", `"pkg/@tool.pre"`},
		{"action", `"replace"`}, {"action", `""`}, {"result", `"future"`},
		{"est_tokens", `-1`}, {"est_tokens", `1.5`}, {"est_tokens", `"12"`},
		{"body_sha256", `"sha256:abc"`}, {"body_sha256", `"` + strings.Repeat("a", 64) + `"`},
		{"body_sha256", `"sha256:` + strings.Repeat("g", 64) + `"`},
		{"body", `"BODY_PROMPT_NATIVE_COMMAND_SECRET"`},
	} {
		t.Run(test.field+"="+test.value, func(t *testing.T) {
			var record map[string]json.RawMessage
			if err := json.Unmarshal(valid, &record); err != nil {
				t.Fatal(err)
			}
			record[test.field] = json.RawMessage(test.value)
			bad, _ := json.Marshal(record)
			assertStatsSkippedLine(t, bad, valid, false)
		})
	}
	for name, bad := range map[string][]byte{
		"empty": {}, "broken": []byte("{broken"), "truncated": valid[:len(valid)-1],
		"null": []byte("null"), "array": []byte("[]"), "trailing JSON": append(append([]byte{}, valid...), valid...),
	} {
		t.Run(name, func(t *testing.T) { assertStatsSkippedLine(t, bad, valid, len(bad) >= 512) })
	}
}

func assertStatsSkippedLine(t *testing.T, bad, valid []byte, oversized bool) {
	t.Helper()
	root := t.TempDir()
	data := append(append(append([]byte{}, bad...), '\n'), valid...)
	putReaderStats(t, root, "session", "main", data)
	var seen []StatsEntry
	q, err := ScanStats(context.Background(), root, func(entry StatsEntry) error { seen = append(seen, entry); return nil })
	want := StatsQuality{Partial: true, InvalidLines: 1}
	if oversized {
		want.InvalidLines, want.OversizedLines = 0, 1
	}
	if err != nil || q != want || len(seen) != 1 || seen[0].Rule != "removed/check" || seen[0].Session != "session" || !seen[0].Time.Equal(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("seen=%+v quality=%+v want=%+v err=%v", seen, q, want, err)
	}
}

func TestScanStatsLineLimitsAndLegacyRecords(t *testing.T) {
	valid := readerStatsLine(t)
	for _, length := range []int{511, 512, 513, 1 << 20} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			line := append(append([]byte{}, valid...), bytes.Repeat([]byte{' '}, length-len(valid))...)
			if length >= 512 {
				assertStatsSkippedLine(t, line, valid, true)
				return
			}
			root := t.TempDir()
			putReaderStats(t, root, "session", "main", append(line, '\n'))
			seen := 0
			q, err := ScanStats(context.Background(), root, func(StatsEntry) error { seen++; return nil })
			if err != nil || q != (StatsQuality{}) || seen != 1 {
				t.Fatalf("511 bytes: q=%+v seen=%d err=%v", q, seen, err)
			}
		})
	}
	for _, omitted := range [][]string{{"est_tokens"}, {"body_sha256"}, {"est_tokens", "body_sha256"}} {
		var fields map[string]json.RawMessage
		json.Unmarshal(valid, &fields)
		for _, name := range omitted {
			delete(fields, name)
		}
		data, _ := json.Marshal(fields)
		root := t.TempDir()
		putReaderStats(t, root, "legacy", "child", data)
		seen := 0
		q, err := ScanStats(context.Background(), root, func(StatsEntry) error { seen++; return nil })
		if err != nil || q.Partial || seen != 1 {
			t.Fatalf("legacy %v: q=%+v seen=%d err=%v", omitted, q, seen, err)
		}
	}
}

func TestScanStatsDepthAndSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	valid := readerStatsLine(t)
	putReaderStats(t, root, "one", "main", valid)
	external := putReaderStats(t, outside, "external", "main", valid)
	for link, target := range map[string]string{
		filepath.Join(root, "session-link"):             outside,
		filepath.Join(root, "one", "agent-link"):        filepath.Dir(external),
		filepath.Join(root, "one", "main", "file-link"): external,
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	// A stats symlink itself must also be skipped, including a broken target.
	for name, target := range map[string]string{"linked": external, "broken": filepath.Join(outside, "missing")} {
		path := filepath.Join(root, "one", name)
		os.MkdirAll(path, 0o700)
		if err := os.Symlink(target, filepath.Join(path, "stats.jsonl")); err != nil {
			t.Fatal(err)
		}
	}
	// Files at the wrong depth and subdirectories below agent level are ignored.
	os.WriteFile(filepath.Join(root, "stats.jsonl"), valid, 0o600)
	os.WriteFile(filepath.Join(root, "one", "stats.jsonl"), valid, 0o600)
	putReaderStats(t, filepath.Join(root, "one", "main", "deep"), "x", "y", valid)
	seen := 0
	q, err := ScanStats(context.Background(), root, func(StatsEntry) error { seen++; return nil })
	if err != nil || seen != 1 || q != (StatsQuality{Partial: true, SkippedSymlinks: 5}) {
		t.Fatalf("seen=%d q=%+v err=%v", seen, q, err)
	}
	for _, invalidRoot := range []string{filepath.Join(root, "session-link")} {
		if _, err := ScanStats(context.Background(), invalidRoot, func(StatsEntry) error { t.Fatal("followed root symlink"); return nil }); err == nil {
			t.Fatal("accepted symlink root")
		}
	}
	if _, err := ScanStats(context.Background(), external, func(StatsEntry) error { return nil }); err == nil {
		t.Fatal("accepted file root")
	}
}

func TestScanStatsMissingDisappearingPermissionAndCancellation(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	q, err := ScanStats(context.Background(), missing, func(StatsEntry) error { return nil })
	if err != nil || q.Partial {
		t.Fatalf("missing root: %+v %v", q, err)
	}
	if _, err := os.Lstat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("created root: %v", err)
	}
	root := t.TempDir()
	paths := []string{}
	for _, agent := range []string{"main", "child-1", "child-2"} {
		paths = append(paths, putReaderStats(t, root, "session", agent, readerStatsLine(t)))
	}
	seen := 0
	_, err = ScanStats(context.Background(), root, func(StatsEntry) error {
		seen++
		for _, path := range paths {
			os.Remove(path)
		}
		return nil
	})
	if err != nil || seen != 1 {
		t.Fatalf("disappearing files: seen=%d err=%v", seen, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ScanStats(ctx, missing, func(StatsEntry) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("missing canceled: %v", err)
	}
	valid := append(readerStatsLine(t), '\n')
	putReaderStats(t, root, "session", "main", append(append([]byte{}, valid...), valid...))
	ctx, cancel = context.WithCancel(context.Background())
	seen = 0
	_, err = ScanStats(ctx, root, func(StatsEntry) error { seen++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || seen != 1 {
		t.Fatalf("cancel during read: seen=%d err=%v", seen, err)
	}
	if os.Geteuid() != 0 {
		path := paths[0]
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(path, 0o600) })
		_, err = ScanStats(context.Background(), root, func(StatsEntry) error { return nil })
		if !errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), path) {
			t.Fatalf("permission error: %v", err)
		}
	}
}

type statsFailingReader struct{}

func (statsFailingReader) Read([]byte) (int, error) { return 0, errors.New("SECRET_IO_PATH") }

func TestReadStatsLinesSanitizesIOAndPropagatesVisitorError(t *testing.T) {
	var q StatsQuality
	err := readStatsLines(context.Background(), io.MultiReader(bytes.NewReader(readerStatsLine(t)), statsFailingReader{}), "session", &q, func(StatsEntry) error { t.Fatal("partial I/O record visited"); return nil })
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("I/O error=%v", err)
	}
	sentinel := errors.New("visitor stopped")
	err = readStatsLines(context.Background(), bytes.NewReader(readerStatsLine(t)), "session", &q, func(StatsEntry) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("visitor error=%v", err)
	}
	max := int64(math.MaxInt64)
	if incrementStatsCount(&max) == nil || max != math.MaxInt64 {
		t.Fatal("quality overflow accepted")
	}
}
