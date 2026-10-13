package warn

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FirstSentence returns the first sentence-like segment from the first
// non-empty line of rendered warning text. Chinese sentence-ending marks
// always end the segment; ASCII punctuation ends it only before whitespace or
// the end of the line. If no line has content, FirstSentence returns an empty
// string.
//
// Callers should pass rendered text so dynamic template values participate in
// extraction just like static text.
func FirstSentence(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for offset := 0; offset < len(line); {
			current, size := utf8.DecodeRuneInString(line[offset:])
			end := offset + size
			switch current {
			case '。', '！', '？':
				return line[:end]
			case '.', '!', '?':
				if end == len(line) {
					return line[:end]
				}
				next, _ := utf8.DecodeRuneInString(line[end:])
				if unicode.IsSpace(next) {
					return line[:end]
				}
			}
			offset = end
		}

		return line
	}
	return ""
}
