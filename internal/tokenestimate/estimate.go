// Package tokenestimate contains the STYLE offline token estimate shared by
// validation and the later planning/compilation stages.
package tokenestimate

import "unicode"

// Estimate returns ceil(A/4 + C + 0.5*P), where A is ASCII letters/digits,
// C is CJK Unified Ideographs, kana, or hangul, and P is every other rune.
func Estimate(text string) int {
	ascii, cjk, other := 0, 0, 0
	for _, r := range text {
		switch {
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			ascii++
		case unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana):
			cjk++
		default:
			other++
		}
	}
	// ceil(ascii/4 + cjk + other/2), keeping the calculation integral.
	return (ascii*2 + cjk*8 + other*4 + 7) / 8
}
