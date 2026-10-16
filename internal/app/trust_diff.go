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

// diffContext is the number of unchanged lines shown around each change.
const diffContext = 3

// maxDiffCells bounds the line LCS table. Changed regions larger than this
// are shown as one replacement block, keeping work bounded by file size even
// for adversarial repeated lines.
const maxDiffCells = 1 << 22

type diffLine struct {
	op   byte // ' ', '-' or '+'
	text string
}

// textDiff renders a unified diff with context hunks, so lines that did not
// change between two edits are shown once as context instead of as -/+.
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
	ops := diffLines(lines(before), lines(after))
	var out strings.Builder
	fmt.Fprintf(&out, "--- approved/%s\n+++ current/%s\n", name, name)
	oldLine, newLine := 0, 0
	for i := 0; i < len(ops); {
		if ops[i].op == ' ' {
			oldLine++
			newLine++
			i++
			continue
		}
		// Extend the hunk while the next change is close enough that the
		// context between them would overlap.
		first, last := i, i
		for j := i + 1; j < len(ops) && j <= last+2*diffContext+1; j++ {
			if ops[j].op != ' ' {
				last = j
			}
		}
		start := max(0, first-diffContext)
		end := min(len(ops), last+diffContext+1)
		oldStart, newStart := oldLine-(first-start), newLine-(first-start)
		oldCount, newCount := 0, 0
		for _, line := range ops[start:end] {
			if line.op != '+' {
				oldCount++
			}
			if line.op != '-' {
				newCount++
			}
		}
		if oldCount > 0 {
			oldStart++
		}
		if newCount > 0 {
			newStart++
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
		for _, line := range ops[start:end] {
			out.WriteString(string(line.op) + line.text)
			if !strings.HasSuffix(line.text, "\n") {
				out.WriteString("\n\\ No newline at end of file\n")
			}
		}
		for _, line := range ops[first:end] {
			if line.op != '+' {
				oldLine++
			}
			if line.op != '-' {
				newLine++
			}
		}
		i = end
	}
	return out.String()
}

// diffLines aligns two line lists by their longest common subsequence after
// trimming the common prefix and suffix.
func diffLines(old, current []string) []diffLine {
	prefix := 0
	for prefix < len(old) && prefix < len(current) && old[prefix] == current[prefix] {
		prefix++
	}
	endOld, endNew := len(old), len(current)
	for endOld > prefix && endNew > prefix && old[endOld-1] == current[endNew-1] {
		endOld--
		endNew--
	}
	ops := make([]diffLine, 0, len(old)+len(current))
	for _, line := range old[:prefix] {
		ops = append(ops, diffLine{' ', line})
	}
	a, b := old[prefix:endOld], current[prefix:endNew]
	if (len(a)+1)*(len(b)+1) > maxDiffCells {
		for _, line := range a {
			ops = append(ops, diffLine{'-', line})
		}
		for _, line := range b {
			ops = append(ops, diffLine{'+', line})
		}
	} else {
		width := len(b) + 1
		lcs := make([]int32, (len(a)+1)*width)
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					lcs[i*width+j] = lcs[(i+1)*width+j+1] + 1
				} else {
					lcs[i*width+j] = max(lcs[(i+1)*width+j], lcs[i*width+j+1])
				}
			}
		}
		i, j := 0, 0
		for i < len(a) || j < len(b) {
			switch {
			case i < len(a) && j < len(b) && a[i] == b[j]:
				ops = append(ops, diffLine{' ', a[i]})
				i++
				j++
			case j == len(b) || (i < len(a) && lcs[(i+1)*width+j] >= lcs[i*width+j+1]):
				ops = append(ops, diffLine{'-', a[i]})
				i++
			default:
				ops = append(ops, diffLine{'+', b[j]})
				j++
			}
		}
	}
	for _, line := range old[endOld:] {
		ops = append(ops, diffLine{' ', line})
	}
	return ops
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
