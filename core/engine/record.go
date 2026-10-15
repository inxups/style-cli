package engine

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/inxups/style-cli/core/tokens"
)

func recordCandidate(item candidate, result string) Record {
	digest := sha256.Sum256([]byte(item.modelText))
	return Record{Entry: item.entry.Entry, Action: item.entry.Action, Result: result, Point: item.point.Name,
		EstimatedTokens: tokens.EstimateText(item.modelText).Tokens, BodySHA256: "sha256:" + hex.EncodeToString(digest[:])}
}
