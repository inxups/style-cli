package app

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type FileDiff struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

// One replacement hunk keeps the implementation bounded by file size, even
// for adversarial repeated lines. Common leading and trailing lines are elided.
func textDiff(name string, before, after []byte) string {
	if bytes.Equal(before, after) {
		return ""
	}
	if !utf8.Valid(before) || !utf8.Valid(after) || bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 {
		return fmt.Sprintf("Binary %s: %s -> %s\n", name, hashBytes(before), hashBytes(after))
	}
	lines := func(data []byte) []string {
		if len(data) == 0 {
			return nil
		}
		values := strings.SplitAfter(string(data), "\n")
		if values[len(values)-1] == "" {
			values = values[:len(values)-1]
		}
		return values
	}
	old, current := lines(before), lines(after)
	start := 0
	for start < len(old) && start < len(current) && old[start] == current[start] {
		start++
	}
	endOld, endNew := len(old), len(current)
	for endOld > start && endNew > start && old[endOld-1] == current[endNew-1] {
		endOld--
		endNew--
	}
	oldStart, newStart := start+1, start+1
	if endOld == start {
		oldStart = start
	}
	if endNew == start {
		newStart = start
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- approved/%s\n+++ current/%s\n@@ -%d,%d +%d,%d @@\n", name, name, oldStart, endOld-start, newStart, endNew-start)
	write := func(prefix string, values []string) {
		for _, line := range values {
			out.WriteString(prefix + line)
			if !strings.HasSuffix(line, "\n") {
				out.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	write("-", old[start:endOld])
	write("+", current[start:endNew])
	return out.String()
}

func diffMaps(before, after map[string][]byte) []FileDiff {
	names := make(map[string]bool)
	for name := range before {
		names[name] = true
	}
	for name := range after {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	result := make([]FileDiff, 0)
	for _, name := range ordered {
		_, oldExists := before[name]
		_, newExists := after[name]
		diff := textDiff(name, before[name], after[name])
		if diff == "" && oldExists != newExists {
			diff = fmt.Sprintf("Empty file %s: present=%t -> present=%t\n", name, oldExists, newExists)
		}
		if diff != "" {
			result = append(result, FileDiff{Path: name, Diff: diff})
		}
	}
	return result
}
