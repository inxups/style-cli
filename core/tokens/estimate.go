package tokens

import "github.com/inxups/style-cli/internal/tokenestimate"

// EstimateText counts Unicode characters and estimates STYLE input tokens.
func EstimateText(text string) Estimate {
	count := tokenestimate.CountText(text)
	return Estimate{Tokens: count.Tokens, Chars: count.Chars}
}
