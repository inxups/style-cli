package spec

type validator struct {
	pkg             *Package
	diagnostics     []Diagnostic
	declaredReads   map[string]bool
	declaredHarness map[string]bool
	usedReads       map[string]bool
}

// Validate checks a loaded package and returns stable, source-associated
// diagnostics. Load should be called first to report parse and filesystem
// errors with their original codes.
func Validate(pkg *Package) []Diagnostic {
	if pkg == nil {
		return []Diagnostic{{Code: "invalid-package", Message: "package is nil", Severity: SeverityError}}
	}
	v := &validator{
		pkg:             pkg,
		declaredReads:   make(map[string]bool),
		declaredHarness: make(map[string]bool),
		usedReads:       make(map[string]bool),
	}
	v.validatePackage()
	v.collectReads()
	v.validateRules()
	v.reportUnusedReads()
	v.validateUndeclaredRuleFiles()
	return v.diagnostics
}

func (v *validator) add(code, source, message string) {
	v.diagnostics = append(v.diagnostics, Diagnostic{Code: code, Path: source, Message: message, Severity: SeverityError})
}

func (v *validator) warn(code, source, message string) {
	v.diagnostics = append(v.diagnostics, Diagnostic{Code: code, Path: source, Message: message, Severity: SeverityWarning})
}
