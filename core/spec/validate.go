package spec

type validator struct {
	pkg             *Package
	harnesses       map[string]bool // nil means structure/lint validation only
	diagnostics     []Diagnostic
	declaredReads   map[string]bool
	declaredHarness map[string]bool
	usedReads       map[string]bool
}

// ValidationCatalog supplies the registered canonical harness names and the
// native hook surfaces supported by a subset of those harnesses. A zero
// catalog has no registered harnesses; Native cannot add names to Harnesses.
type ValidationCatalog struct {
	Harnesses []string
	Native    NativeCatalog
}

// Validate checks structure, types, reads, templates and lints, returning
// stable, source-associated diagnostics. It does not check harness membership
// or native capabilities. Package admission must use ValidateWithCatalog.
// Load should be called first to report parse and filesystem errors.
func Validate(pkg *Package) []Diagnostic {
	return validate(pkg, nil)
}

// ValidateWithCatalog performs complete validation using the explicitly
// registered harnesses, independent of detection or command-line selection.
func ValidateWithCatalog(pkg *Package, catalog ValidationCatalog) []Diagnostic {
	harnesses := stringSet(catalog.Harnesses...)
	diagnostics := validate(pkg, harnesses)
	return append(diagnostics, validateNative(pkg, catalog.Native, harnesses)...)
}

func validate(pkg *Package, harnesses map[string]bool) []Diagnostic {
	if pkg == nil {
		return []Diagnostic{{Code: "invalid-package", Message: "package is nil", Severity: SeverityError}}
	}
	v := &validator{
		pkg:             pkg,
		harnesses:       harnesses,
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
