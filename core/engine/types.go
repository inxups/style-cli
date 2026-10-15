package engine

import (
	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/when"
)

// Capabilities are the output channels supported at one evaluation point.
type Capabilities struct {
	AddContext    bool
	SystemMessage bool
	Deny          bool
}

// Point contains one normalized evaluation point and its provider facts.
type Point struct {
	Name          string
	CanonicalTool string
	ToolName      string
	MCPServer     string
	MCPTool       string
	SkillName     string
	When          when.Input
	Values        map[string]string
	Capabilities  Capabilities
}

// Input contains provider-neutral facts for one hook invocation.
type Input struct {
	Harness   string
	Source    string
	TurnID    string
	Scope     state.Scope
	Variables map[string]string
	Points    []Point
}

// Output exposes only channels permitted by STYLE runtime output policy.
type Output struct {
	Block                  bool
	Reason                 string
	AdditionalContext      string
	SystemMessage          string
	SuppressOriginalPrompt bool
	Records                []Record
	Reservations           []*state.Reservation
}

// Record contains safe telemetry without rendered rule or event content.
type Record struct {
	Entry           string
	Action          string
	Result          string
	Point           string
	EstimatedTokens int
	BodySHA256      string
}

type candidate struct {
	entry       compile.Entry
	point       Point
	modelText   string
	userText    string
	reservation *state.Reservation
}

type compiledEntry struct {
	entry   compile.Entry
	matcher *when.Matcher
}
