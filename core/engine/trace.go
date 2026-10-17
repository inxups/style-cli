package engine

import (
	"crypto/sha256"
	"fmt"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/when"
)

// Trace is invocation scoped and contains no event or rendered text.
type Trace struct {
	Points  []PointTrace  `json:"points"`
	Entries []*EntryTrace `json:"entries"`
}
type PointTrace struct {
	Point          string `json:"point"`
	CandidateCount int    `json:"candidate_count"`
}
type RenderSummary struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type EntryTrace struct {
	Rule                string        `json:"rule"`
	Entry               string        `json:"entry"`
	Point               string        `json:"point"`
	Order               compile.Order `json:"order"`
	Action              string        `json:"action"`
	Needs               []string      `json:"needs"`
	Source              string        `json:"source"`
	Selector            string        `json:"selector"`
	Sources             string        `json:"sources"`
	When                string        `json:"when"`
	Conditions          []when.Check  `json:"conditions"`
	Render              string        `json:"render"`
	Model               RenderSummary `json:"model"`
	User                RenderSummary `json:"user"`
	Block               string        `json:"block"`
	Repeat              string        `json:"repeat"`
	RepeatKeySHA256     string        `json:"repeat_key_sha256,omitempty"`
	Generation          uint64        `json:"gen"`
	Epoch               uint64        `json:"epoch"`
	TurnSource          string        `json:"turn_source,omitempty"`
	Output              string        `json:"output"`
	ReservationReleased bool          `json:"reservation_released"`
}

func newEntryTrace(e compile.Entry, point string) *EntryTrace {
	return &EntryTrace{Rule: e.Rule, Entry: e.Entry, Point: point, Order: e.Order, Action: e.Action, Needs: append([]string{}, e.Needs...), Source: e.Source, Selector: "not-evaluated", Sources: "not-evaluated", When: "not-evaluated", Conditions: []when.Check{}, Render: "not-evaluated", Block: "not-evaluated", Repeat: "not-evaluated", Output: "not-evaluated"}
}
func renderSummary(text string) RenderSummary {
	return RenderSummary{len(text), fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(text)))}
}
