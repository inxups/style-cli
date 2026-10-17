package compile

import (
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/inxups/style-cli/core/warn"
	"github.com/inxups/style-cli/core/when"
)

const (
	// Format is the stable on-disk index format emitted by M1.
	Format        = 1
	RefreshFormat = 2
	EventFormat   = 3
)

// PackageInfo identifies one installed package in the index. Seq is the
// installation sequence used as the middle component of rule ordering.
type PackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Seq     int    `json:"seq"`
	Lock    string `json:"lock"`
	Link    bool   `json:"link"`
}

// Index is the complete runtime index for all installed packages and harnesses.
type Index struct {
	Format       int                           `json:"format"`
	StyleVersion string                        `json:"style_version"`
	Packages     []PackageInfo                 `json:"packages"`
	Static       map[string][]StaticEntry      `json:"static"`
	Harnesses    map[string]map[string][]Entry `json:"harnesses"`
}

// StaticEntry describes a rule written to a provider-owned static location.
type StaticEntry struct {
	Rule       string          `json:"rule"`
	Slot       string          `json:"slot"`
	Source     string          `json:"src"`
	BodySHA256 string          `json:"body_sha256"`
	Estimate   tokens.Estimate `json:"est"`
}

// Entry is one runtime evaluation point. The body is represented as static
// text or literal/variable fragments so the hot path never reparses Markdown.
type Entry struct {
	Refresh                int             `json:"refresh,omitempty"`
	Rule                   string          `json:"rule"`
	Entry                  string          `json:"entry"`
	Source                 string          `json:"src"`
	SourceSHA256           string          `json:"src_sha256"`
	BodySHA256             string          `json:"body_sha256"`
	Order                  Order           `json:"order"`
	Target                 string          `json:"target"`
	Action                 string          `json:"action"`
	Match                  *Match          `json:"match,omitempty"`
	When                   when.Rule       `json:"when"`
	Repeat                 string          `json:"repeat"`
	ReinjectOnCompact      bool            `json:"reinject_on_compact"`
	Sources                *[]string       `json:"sources,omitempty"`
	Reevaluate             bool            `json:"reeval,omitempty"`
	Needs                  []string        `json:"needs"`
	Body                   Body            `json:"body"`
	ModelBody              *Body           `json:"model_body,omitempty"`
	ModelBodyFirstSentence bool            `json:"model_body_first_sentence,omitempty"`
	Estimate               tokens.Estimate `json:"est"`
	DegradedFrom           *string         `json:"degraded_from"`
}

// Order is serialized as [priority, package sequence, rule index]. Priority
// is numeric in STYLE and may be fractional; the other two elements are ints.
type Order struct {
	Priority   float64
	PackageSeq int
	RuleIndex  int
}

// Match is the normalized selector set used before evaluating when.
type Match struct {
	Canonical []string      `json:"canonical,omitempty"`
	Names     []string      `json:"names,omitempty"`
	MCP       []MCPSelector `json:"mcp,omitempty"`
	Skill     []string      `json:"skill,omitempty"`
}

// MCPSelector is a normalized server/tool selector. Either component may be
// "*" according to the STYLE matching rules.
type MCPSelector struct {
	Server string `json:"server"`
	Tool   string `json:"tool"`
}

// Variable is a runtime template value. A nil Default means no default was
// declared; a pointer preserves an explicitly declared empty default.
type Variable struct {
	Name    string
	Default *string
}

// Fragment is either literal text or one runtime variable. Exactly one of
// Text and Variable is populated.
type Fragment struct {
	Text     string
	Variable *Variable
}

// Body is encoded as {"static":"..."} or {"parts":[...]} to match the
// STYLE index format while keeping the in-memory representation type-safe.
type Body struct {
	Static *string
	Parts  []Fragment
}

// PackageInput supplies an installed package, its lock identity, per-harness
// plans, and values collected during installation.
type PackageInput struct {
	Package  *spec.ResolvedPackage
	Sequence int
	Lock     string
	Link     bool
	Settings map[string]string
	Plans    []plan.Report
}

// Request supplies the pure inputs needed for one complete index compile.
type Request struct {
	StyleVersion string
	Packages     []PackageInput
}

// StaticBody is installation output kept outside index.json. Static entries
// in the index contain only metadata; routes receive the rendered text here.
type StaticBody struct {
	Harness string
	Rule    string
	Slot    string
	Mode    string
	Body    string
	Order   Order
}

// Result contains the runtime index and the rendered static installation text.
type Result struct {
	Index        Index
	StaticBodies []StaticBody
	// Deduplicated and DeduplicationWarnings describe cold-path decisions.
	// Neither is part of the runtime index or persistent repeat identity.
	Deduplicated          []Deduplication
	DeduplicationWarnings []DeduplicationWarning
}

// Deduplication folds every placement of Rule into one canonical rule.
type Deduplication struct {
	Harness    string
	Rule       string
	Into       string
	Placements []PlacementRef
}

type PlacementRef struct {
	Event      string
	StaticSlot string
}

type DeduplicationWarning struct {
	Harness string
	Rule    string
	Reason  string
}

// RenderModelText renders the model-facing text, applying warn's implicit
// first-sentence rule after runtime values have been substituted.
func (entry Entry) RenderModelText(values map[string]string) string {
	body := entry.Body
	if entry.ModelBody != nil {
		body = *entry.ModelBody
	}
	text := body.Render(values)
	if entry.ModelBodyFirstSentence {
		return warn.FirstSentence(text)
	}
	return text
}
