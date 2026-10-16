package app

import (
	"fmt"
	"strings"
	"testing"
)

func numberedLines(count int, change map[int]string) string {
	var out strings.Builder
	for i := 1; i <= count; i++ {
		if text, ok := change[i]; ok {
			out.WriteString(text + "\n")
			continue
		}
		fmt.Fprintf(&out, "line %d\n", i)
	}
	return out.String()
}

// M1B-TRUST-01: two separate edits produce two context hunks; the unchanged
// lines between them are not repeated as removed and added.
func TestTextDiffShowsSeparateContextHunks(t *testing.T) {
	before := numberedLines(20, nil)
	after := numberedLines(20, map[int]string{2: "changed 2", 18: "changed 18"})
	want := `--- approved/a.md
+++ current/a.md
@@ -1,5 +1,5 @@
 line 1
-line 2
+changed 2
 line 3
 line 4
 line 5
@@ -15,6 +15,6 @@
 line 15
 line 16
 line 17
-line 18
+changed 18
 line 19
 line 20
`
	if got := textDiff("a.md", []byte(before), []byte(after)); got != want {
		t.Fatalf("textDiff:\n%s\nwant:\n%s", got, want)
	}
}

func TestTextDiffMergesNearbyChangesAndAlignsInsertions(t *testing.T) {
	before := "a\nb\nc\nd\ne\nf\ng\nh\n"
	after := "a\nB\nc\nd\nx\ne\nf\nG\nh\n"
	want := `--- approved/f
+++ current/f
@@ -1,8 +1,9 @@
 a
-b
+B
 c
 d
+x
 e
 f
-g
+G
 h
`
	if got := textDiff("f", []byte(before), []byte(after)); got != want {
		t.Fatalf("textDiff:\n%s\nwant:\n%s", got, want)
	}
}

func TestTextDiffEdgeCases(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		want          string
	}{
		{"new file", "", "a\nb\n", "--- approved/f\n+++ current/f\n@@ -0,0 +1,2 @@\n+a\n+b\n"},
		{"removed file", "a\n", "", "--- approved/f\n+++ current/f\n@@ -1,1 +0,0 @@\n-a\n"},
		{"missing final newline", "a\nb", "a\nc", "--- approved/f\n+++ current/f\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := textDiff("f", []byte(tc.before), []byte(tc.after)); got != tc.want {
				t.Fatalf("textDiff:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestTextDiffFallsBackToOneBlockForHugeChanges(t *testing.T) {
	var before, after strings.Builder
	for i := 0; i < 2100; i++ {
		fmt.Fprintf(&before, "old %d\n", i)
		fmt.Fprintf(&after, "new %d\n", i)
	}
	got := textDiff("big", []byte(before.String()), []byte(after.String()))
	if !strings.HasPrefix(got, "--- approved/big\n+++ current/big\n@@ -1,2100 +1,2100 @@\n-old 0\n") || !strings.Contains(got, "-old 2099\n+new 0\n") {
		t.Fatalf("huge change did not fall back to one replacement block: %.200q", got)
	}
}
