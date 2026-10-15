// Package spec loads STYLE directory and single-file packages.
package spec

import "errors"

const MaxFileSize = 256 * 1024

// Package is the parsed, unmerged representation of a STYLE package.
// Manifest and Rule.Fields retain source values so later validation and merge
// stages can distinguish declared fields from defaults.
type Package struct {
	Root       string
	Manifest   map[string]any
	Metadata   map[string]any
	Rules      []Rule
	Files      map[string][]byte
	SingleFile bool
	inventory  []string // Non-nil for a complete in-memory snapshot.
}

// Rule is one entry in rules[], or the sole rule in a single-file package.
type Rule struct {
	Index      int
	Source     string
	From       string
	Fields     map[string]any
	Overrides  map[string]any
	Effective  map[string]any
	Body       string
	BodySource string
}

// ResolvedPackage contains the enabled rules after package and rule defaults
// have been merged. Package retains the original parsed values for validation.
type ResolvedPackage struct {
	Package *Package
	Reads   map[string]any
	Rules   []Rule
}

// ParseError carries a stable machine-readable code for CLI diagnostics.
type ParseError struct {
	Code string
	Path string
	Msg  string
	Err  error
}

// Diagnostic is a stable, source-associated validation result. Warnings do
// not prevent a package from being used; errors do.
type Diagnostic struct {
	Code     string
	Path     string
	Message  string
	Severity string
}

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

func (e *ParseError) Error() string {
	where := ""
	if e.Path != "" {
		where = e.Path + ": "
	}
	if e.Err != nil {
		return where + e.Msg + ": " + e.Err.Error()
	}
	return where + e.Msg
}

func (e *ParseError) Unwrap() error { return e.Err }

func (e *ParseError) ErrorCode() string { return e.Code }

func (d Diagnostic) Error() string {
	where := ""
	if d.Path != "" {
		where = d.Path + ": "
	}
	return where + d.Message
}

func (d Diagnostic) ErrorCode() string { return d.Code }

// ErrorCode returns the parse or validation code embedded in err.
func ErrorCode(err error) string {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return parseErr.Code
	}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		return coded.ErrorCode()
	}
	return ""
}

func parseErr(code, source, message string, err error) error {
	return &ParseError{Code: code, Path: source, Msg: message, Err: err}
}
