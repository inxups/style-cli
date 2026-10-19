package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/testfs"
)

func statsClock() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) }

func statsFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	source := filepath.Join(repositoryRoot(t), "testdata", "stats", "m2")
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "stats.jsonl" {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(home, "state", rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o640)
	})
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func TestA5StatsRuleAndSessionGoldens(t *testing.T) {
	home := statsFixture(t)
	t.Setenv("STYLE_HOME", home)
	// No adapter registry, lock, package, or index is needed to read history.
	service, err := New()
	if err != nil {
		t.Fatal(err)
	}
	wantRules := []StatsRow{
		{Provider: "codex", Rule: "pkg/check", StatsCounts: StatsCounts{Outputs: 6, Dedup: 1, CapDrops: 1, EstimatedTokens: 37, Unestimated: 1}},
		{Provider: "claude-code", Rule: "pkg/check", StatsCounts: StatsCounts{Outputs: 1, Blocks: 1, EstimatedTokens: 27}},
		{Provider: "codex", Rule: "pkg/stop", StatsCounts: StatsCounts{Blocks: 1, EstimatedTokens: 12}},
		{Provider: "codex", Rule: "pkg/boundary", StatsCounts: StatsCounts{Outputs: 1, EstimatedTokens: 10}},
		{Provider: "codex", Rule: "pkg/else", StatsCounts: StatsCounts{Outputs: 1, EstimatedTokens: 10}},
		{Provider: "codex", Rule: "removed/legacy", StatsCounts: StatsCounts{Outputs: 1, Blocks: 1, Unestimated: 2}},
		{Provider: "retired-harness", Rule: "removed/free", StatsCounts: StatsCounts{Outputs: 1}},
	}
	wantSessions := []StatsRow{
		{Provider: "codex", Session: "alpha", StatsCounts: StatsCounts{Outputs: 6, Blocks: 1, Dedup: 1, CapDrops: 1, EstimatedTokens: 47, Unestimated: 2}},
		{Provider: "claude-code", Session: "alpha", StatsCounts: StatsCounts{Outputs: 1, Blocks: 1, EstimatedTokens: 27}},
		{Provider: "codex", Session: "beta", StatsCounts: StatsCounts{Outputs: 3, Blocks: 1, EstimatedTokens: 22, Unestimated: 1}},
		{Provider: "retired-harness", Session: "alpha", StatsCounts: StatsCounts{Outputs: 1}},
	}
	for _, by := range []string{"rule", "session"} {
		result, err := statsAt(context.Background(), StatsRequest{Home: home, By: by}, statsClock())
		if err != nil {
			t.Fatal(err)
		}
		want := wantRules
		if by == "session" {
			want = wantSessions
		}
		if !reflect.DeepEqual(result.Rows, want) || result.Totals != (StatsCounts{Outputs: 11, Blocks: 3, Dedup: 1, CapDrops: 1, EstimatedTokens: 96, Unestimated: 3}) || result.Quality.Partial {
			t.Fatalf("%s: %+v\nwant rows %+v", by, result, want)
		}
		for _, format := range []string{"txt", "json"} {
			args := []string{"--by", by}
			if format == "json" {
				args = append(args, "--json")
			}
			var out, errOut bytes.Buffer
			if code := runStatsAt(context.Background(), args, &out, &errOut, statsClock()); code != 0 || errOut.Len() != 0 {
				t.Fatalf("%v: exit=%d stderr=%s", args, code, &errOut)
			}
			golden := filepath.Join(repositoryRoot(t), "testdata", "golden", "stats-"+by+"."+format)
			if os.Getenv("STYLE_UPDATE_GOLDENS") == "1" {
				if err := os.WriteFile(golden, out.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := os.ReadFile(golden)
			if err != nil || !bytes.Equal(out.Bytes(), expected) {
				t.Fatalf("%s mismatch: err=%v\n%s", golden, err, &out)
			}
		}
	}
	// Exercise the real process clock with a recent record, independently of the
	// fixed-clock golden fixture. An empty registry still accepts old providers.
	liveHome := t.TempDir()
	live := appStatsLine(t, "retired-harness", 12)
	live = bytes.ReplaceAll(live, []byte(statsClock().Format(time.RFC3339Nano)), []byte(time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)))
	putAppStats(t, liveHome, live)
	t.Setenv("STYLE_HOME", liveHome)
	var out, errOut bytes.Buffer
	if code := RunWithService(context.Background(), service, home, []string{"stats", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("dispatch exit=%d stderr=%s", code, &errOut)
	}
	var report StatsResult
	if err := decodeReport(out.Bytes(), &report); err != nil || report.Format != 1 || len(report.Rows) != 1 || report.Rows[0].Provider != "retired-harness" {
		t.Fatalf("dispatch JSON: %v %s", err, &out)
	}
	if liveResult, err := service.Stats(context.Background(), StatsRequest{Home: liveHome}); err != nil || liveResult.Totals.Outputs != 1 {
		t.Fatalf("service stats: %+v %v", liveResult, err)
	}
	costs, legacy, err := readRuleCosts(context.Background(), filepath.Join(home, "state"), statsClock())
	wantCosts := []RuleCost{
		{Provider: "codex", Rule: "pkg/check", EstimatedTokens: 37, Emits: 5, DistinctBodyHashes: 1, RepeatedBody: true},
		{Provider: "claude-code", Rule: "pkg/check", EstimatedTokens: 27, Emits: 2, DistinctBodyHashes: 1},
		{Provider: "codex", Rule: "pkg/stop", EstimatedTokens: 12, Emits: 1, DistinctBodyHashes: 1},
	}
	if err != nil || legacy != 3 || !reflect.DeepEqual(costs, wantCosts) {
		t.Fatalf("doctor compatibility: %+v legacy=%d err=%v", costs, legacy, err)
	}
}

func TestA5StatsWindowsAndUsage(t *testing.T) {
	home := statsFixture(t)
	t.Setenv("STYLE_HOME", home)
	now := statsClock()
	for _, test := range []struct {
		since string
		want  int64
	}{
		{"", 96}, {"168h", 96}, {"24h", 94}, {"30m", 10},
		{"2026-10-09T09:00:00+09:00", 96}, {"2026-10-16T00:00:00Z", 10},
	} {
		result, err := statsAt(context.Background(), StatsRequest{Since: test.since}, now)
		if err != nil || result.Totals.EstimatedTokens != test.want || result.Window.Since.Location() != time.UTC || result.Window.Until.Location() != time.UTC {
			t.Fatalf("since=%q: %+v err=%v", test.since, result, err)
		}
	}
	for _, args := range [][]string{
		{"--by", "other"}, {"--by"}, {"--by="}, {"--by", ""}, {"--since"}, {"--since="},
		{"--since", "7d"}, {"--since", "0"}, {"--since=-1h"}, {"--since", "2030-01-01T00:00:00Z"},
		{"--since", "2026-10-16T00:00:00.000000001Z"}, {"--since", "2026-10-09"},
		{"--unknown"}, {"positional"}, {"--json=true"},
	} {
		var out, errOut bytes.Buffer
		code := runStatsAt(context.Background(), args, &out, &errOut, now)
		if code != ExitUsage || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("args=%v: exit=%d stdout=%s stderr=%s", args, code, &out, &errOut)
		}
	}
	for _, by := range []string{"rule", "session"} {
		for _, args := range [][]string{{"--by", by, "--since", "24h", "--json"}, {"--by=" + by, "--since=24h", "--json"}} {
			var out, errOut bytes.Buffer
			if code := runStatsAt(context.Background(), args, &out, &errOut, now); code != 0 {
				t.Fatalf("%v: %d %s", args, code, &errOut)
			}
		}
	}
	var out, errOut bytes.Buffer
	if code := runStatsAt(context.Background(), []string{"--help"}, &out, &errOut, now); code != 0 || out.String() != statsCommand.usage()+"\n" || errOut.Len() != 0 {
		t.Fatalf("help: %d %s %s", code, &out, &errOut)
	}
}

func appStatsLine(t *testing.T, provider string, tokens int) []byte {
	t.Helper()
	record := state.StatsRecord{At: statsClock().Format(time.RFC3339Nano), Provider: provider, Event: "PreToolUse", Point: "tool.pre", Entry: "pkg/check@tool.pre", Action: "inject", Result: "output", Tokens: &tokens, BodySHA256: "sha256:" + strings.Repeat("a", 64)}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func putAppStats(t *testing.T, home string, data []byte) string {
	t.Helper()
	path := filepath.Join(home, "state", "session", "main", "stats.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestA5StatsQualityPrivacyAndDoctorContinues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STYLE_HOME", home)
	valid := appStatsLine(t, "codex", 12)
	data := "{BODY_PROMPT_NATIVE_COMMAND_SECRET\n\n" + `{"body":"BODY_PROMPT_NATIVE_COMMAND_SECRET"}` + "\n" + string(valid) + "\n" + strings.Repeat("BODY_PROMPT_NATIVE_COMMAND_SECRET", 40000) + "\n" + string(valid) + "\n" + `{"at":`
	putAppStats(t, home, []byte(data))
	for _, args := range [][]string{nil, {"--json"}} {
		var out, errOut bytes.Buffer
		code := runStatsAt(context.Background(), args, &out, &errOut, statsClock())
		if code != 0 || errOut.Len() != 0 || strings.Contains(out.String()+errOut.String(), "SECRET") {
			t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &errOut)
		}
		if len(args) != 0 {
			var result StatsResult
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Totals.Outputs != 2 || result.Totals.EstimatedTokens != 24 || result.Quality != (state.StatsQuality{Partial: true, InvalidLines: 4, OversizedLines: 1}) {
				t.Fatalf("report=%+v err=%v", result, err)
			}
		} else if !strings.Contains(out.String(), "Quality: partial=true invalid_lines=4 oversized_lines=1 skipped_symlinks=0") {
			t.Fatalf("missing quality text: %s", &out)
		}
	}
	service, _ := New()
	result, err := service.Doctor(context.Background(), DoctorRequest{Home: home})
	if err != nil || !doctorHas(result, "stats-invalid") || len(result.TopRules) != 1 || result.TopRules[0].EstimatedTokens != 24 || result.TopRules[0].Emits != 2 {
		t.Fatalf("doctor=%+v err=%v", result, err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Message, "SECRET") {
			t.Fatal("doctor leaked raw stats")
		}
	}
	// Quality counters are independent of the requested time window.
	filtered, err := statsAt(context.Background(), StatsRequest{Home: home, Since: "1ns"}, statsClock().Add(time.Hour))
	if err != nil || len(filtered.Rows) != 0 || filtered.Quality.InvalidLines != 4 {
		t.Fatalf("window quality: %+v %v", filtered, err)
	}
	putAppStats(t, home, appStatsLine(t, "codex\n\t\x1b[31m", 12))
	var out, errOut bytes.Buffer
	if code := runStatsAt(context.Background(), nil, &out, &errOut, statsClock()); code != 0 || strings.Contains(out.String(), "codex\n\t\x1b") || !strings.Contains(out.String(), `codex\n\t\x1b[31m`) {
		t.Fatalf("unsafe identifiers: exit=%d stdout=%q stderr=%s", code, out.String(), &errOut)
	}
}

type statsFileSnapshot struct {
	Mode os.FileMode
	Time time.Time
	Data string
}

func snapshotStatsTree(t *testing.T, root string) map[string]statsFileSnapshot {
	t.Helper()
	result := map[string]statsFileSnapshot{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			file, e := os.Open(path)
			if e != nil {
				return e
			}
			info, e = file.Stat()
			file.Close()
			if e != nil {
				return e
			}
		}
		item := statsFileSnapshot{Mode: info.Mode(), Time: info.ModTime()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.Data = string(data)
		}
		result[path] = item
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestA5StatsIsReadOnlyAndEmptyReportsDoNotCreateState(t *testing.T) {
	home := statsFixture(t)
	t.Setenv("STYLE_HOME", home)
	// Deliberately invalid installation metadata would fail if stats loaded it.
	if err := os.WriteFile(filepath.Join(home, "style.lock.yaml"), []byte("[invalid lock"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "build", "index.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	// Every directory and file is expired. Stats must not run doctor/sync cleanup.
	if err := filepath.WalkDir(home, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, statsClock().Add(-30*24*time.Hour), statsClock().Add(-30*24*time.Hour))
	}); err != nil {
		t.Fatal(err)
	}
	before := snapshotStatsTree(t, home)
	for _, args := range [][]string{nil, {"--json"}, {"--by=session"}, {"--by=session", "--json"}} {
		if code := runStatsAt(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}, statsClock()); code != 0 {
			t.Fatalf("exit=%d", code)
		}
	}
	if after := snapshotStatsTree(t, home); !reflect.DeepEqual(before, after) {
		t.Fatal("stats changed tree bytes, modes, mtimes or expired sessions")
	}
	for _, home := range []string{filepath.Join(t.TempDir(), "missing-home"), t.TempDir()} {
		t.Setenv("STYLE_HOME", home)
		var out, errOut bytes.Buffer
		if code := runStatsAt(context.Background(), []string{"--json"}, &out, &errOut, statsClock()); code != 0 || errOut.Len() != 0 {
			t.Fatalf("empty: exit=%d stderr=%s", code, &errOut)
		}
		var result StatsResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Rows == nil || len(result.Rows) != 0 || result.Totals != (StatsCounts{}) || result.Quality.Partial {
			t.Fatalf("empty result=%+v err=%v", result, err)
		}
		if _, err := os.Lstat(filepath.Join(home, "state")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("created state: %v", err)
		}
		out.Reset()
		if code := runStatsAt(context.Background(), nil, &out, &errOut, statsClock()); code != 0 || !strings.Contains(out.String(), "No runtime records in window.") {
			t.Fatalf("empty text=%s exit=%d", &out, code)
		}
	}
}

func TestA5StatsFatalErrorsHaveNoSuccessOutput(t *testing.T) {
	for _, scenario := range []string{"file-root", "symlink-root", "permission", "overflow-row", "overflow-totals", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("STYLE_HOME", home)
			ctx := context.Background()
			switch scenario {
			case "file-root":
				os.WriteFile(filepath.Join(home, "state"), []byte("ROOT_SECRET"), 0o600)
			case "symlink-root":
				os.Symlink(t.TempDir(), filepath.Join(home, "state"))
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses read permissions")
				}
				path := putAppStats(t, home, appStatsLine(t, "codex", 12))
				readable := testfs.ReadableFile(t, path)
				readable(false)
			case "overflow-row", "overflow-totals":
				if int64(int(math.MaxInt64)) != math.MaxInt64 {
					t.Skip("log token field uses platform int")
				}
				first := appStatsLine(t, "codex", math.MaxInt64)
				provider := "codex"
				if scenario == "overflow-totals" {
					provider = "other"
				}
				putAppStats(t, home, append(append(first, '\n'), appStatsLine(t, provider, 1)...))
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			for _, args := range [][]string{nil, {"--json"}} {
				var out, errOut bytes.Buffer
				code := runStatsAt(ctx, args, &out, &errOut, statsClock())
				if code != 1 || out.Len() != 0 || errOut.Len() == 0 || strings.Contains(errOut.String(), "ROOT_SECRET") {
					t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &errOut)
				}
			}
		})
	}
	// Independently exercise each numeric field's overflow guard.
	for _, counts := range []StatsCounts{
		{Outputs: math.MaxInt64}, {Blocks: math.MaxInt64}, {Dedup: math.MaxInt64},
		{CapDrops: math.MaxInt64}, {EstimatedTokens: math.MaxInt64}, {Unestimated: math.MaxInt64},
	} {
		if err := counts.add(StatsCounts{Outputs: 1, Blocks: 1, Dedup: 1, CapDrops: 1, EstimatedTokens: 1, Unestimated: 1}); err == nil {
			t.Fatal("counter overflow accepted")
		}
	}
}

func TestA5StatsSessionStorageKeysAndSkippedSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STYLE_HOME", home)
	store, err := state.New(filepath.Join(home, "state"))
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"main", "child/unsafe"} {
		if err := store.AppendStats("session/unsafe", agent, appStatsLine(t, "codex", 12)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "state", "external-session")); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runStatsAt(context.Background(), []string{"--by=session", "--json"}, &out, &errOut, statsClock()); code != 0 || errOut.Len() != 0 {
		t.Fatalf("exit=%d stderr=%s", code, &errOut)
	}
	var report StatsResult
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	wantKey := "!" + strings.TrimPrefix(hashBytes([]byte("session/unsafe")), "sha256:")
	if len(report.Rows) != 1 || report.Rows[0].Session != wantKey || report.Totals.Outputs != 2 || report.Totals.EstimatedTokens != 24 || report.Quality != (state.StatsQuality{Partial: true, SkippedSymlinks: 1}) || strings.Contains(out.String(), "session/unsafe") {
		t.Fatalf("session report=%+v", report)
	}
}
