package spec

import (
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// fakeNativeHarness stands in for an adapter: core/spec must not import one.
type fakeNativeHarness struct {
	events    map[string]bool
	allowStar bool
}

func (h fakeNativeHarness) NativeEvent(event string) (bool, bool) {
	matcher, ok := h.events[event]
	return matcher, ok
}

func (h fakeNativeHarness) ValidMatcher(matcher string) bool {
	if h.allowStar && matcher == "*" {
		return true
	}
	_, err := regexp.Compile(matcher)
	return err == nil
}

var testNativeCatalog = NativeCatalog{
	"codex":       fakeNativeHarness{events: map[string]bool{"Stop": false, "UserPromptSubmit": false, "PreToolUse": true}},
	"claude-code": fakeNativeHarness{events: map[string]bool{"Stop": false, "UserPromptSubmit": false, "PreToolUse": true, "PostToolUse": true}, allowStar: true},
}

func validateWithNative(pkg *Package, catalog NativeCatalog) []Diagnostic {
	return append(Validate(pkg), ValidateNative(pkg, catalog)...)
}

func errorCodes(diagnostics []Diagnostic) []string {
	codes := []string{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			codes = append(codes, diagnostic.Code)
		}
	}
	sort.Strings(codes)
	return codes
}

// loadNativeRule writes fields (one YAML key per line) as either an inline
// STYLE.yaml rule or a single-file package and loads it.
func loadNativeRule(t *testing.T, format, fields, body string) *Package {
	t.Helper()
	root := t.TempDir()
	source := root
	if format == "directory" {
		indented := strings.ReplaceAll(strings.TrimSuffix(fields, "\n"), "\n", "\n    ")
		writeFile(t, root, "STYLE.yaml", testManifest("native-rule", "", "  - id: native-rule\n    "+indented+"\n"))
	} else {
		writeFile(t, root, "native-rule.style.md", "---\nspec: style/v0.3\nname: native-rule\nversion: 1.0.0\ndescription: Native rule\n"+fields+"---\n"+body)
		source = filepath.Join(root, "native-rule.style.md")
	}
	pkg, err := Load(source)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestNativeRuleValidationMatchesAcrossPackageFormats(t *testing.T) {
	const stop = "codex: {event: Stop, command: notify}\n"
	cases := []struct {
		name, fields, want string // want is the required error code; "" means valid
	}{
		{"codex only", "target: native\n" + stop, ""},
		{"claude only with matcher and timeout", "target: native\nclaude-code: {event: PostToolUse, matcher: \"^Bash$\", command: \"notify --title claude\", timeout: 10}\n", ""},
		{"both harnesses", "target: native\n" + stop + "claude-code: {event: Stop, command: notify}\n", ""},
		{"claude star matcher", "target: native\nclaude-code: {event: PreToolUse, matcher: \"*\", command: notify}\n", ""},
		{"quoted command", "target: native\ncodex: {event: Stop, command: 'printf \"%s\" ''a b'' | tee -a log && echo \\;'}\n", ""},
		{"timeout bounds", "target: native\ncodex: {event: Stop, command: a, timeout: 1}\nclaude-code: {event: Stop, command: a, timeout: 600}\n", ""},
		{"disabled rule still validated", "target: native\nenabled: false\ncodex: {event: Stop}\n", "native-command"},

		{"no harness", "target: native\n", "native-mapping"},
		{"list mapping", "target: native\ncodex: [Stop]\n", "native-mapping"},
		{"scalar mapping", "target: native\ncodex: notify\n", "native-mapping"},
		{"null mapping", "target: native\ncodex:\n", "native-mapping"},

		{"action", "target: native\naction: warn\n" + stop, "native-field"},
		{"when", "target: native\nwhen: {harness: {name: codex}}\n" + stop, "native-field"},
		{"repeat", "target: native\nrepeat: every\n" + stop, "native-field"},
		{"event field", "target: native\nevent: tool.pre\n" + stop, "native-field"},
		{"unregistered harness", "target: native\ngemini: {event: Stop, command: notify}\n", "native-field"},
		{"async", "target: native\ncodex: {event: Stop, command: notify, async: true}\n", "native-field"},
		{"onFailure", "target: native\nclaude-code: {event: PreToolUse, command: notify, onFailure: block}\n", "native-field"},
		{"type", "target: native\ncodex: {event: Stop, command: notify, type: command}\n", "native-field"},
		{"args", "target: native\nclaude-code: {event: Stop, command: notify, args: [a]}\n", "native-field"},

		{"missing event", "target: native\ncodex: {command: notify}\n", "native-event"},
		{"non-string event", "target: native\ncodex: {event: 3, command: notify}\n", "native-event"},
		{"codex unknown event", "target: native\ncodex: {event: UserPromptExpansion, command: notify}\n", "native-event"},
		{"claude unknown event", "target: native\nclaude-code: {event: PostModelSwitch, command: notify}\n", "native-event"},

		{"empty matcher", "target: native\ncodex: {event: PreToolUse, matcher: \"\", command: notify}\n", "native-matcher"},
		{"padded matcher", "target: native\ncodex: {event: PreToolUse, matcher: \" Bash\", command: notify}\n", "native-matcher"},
		{"matcher on Stop", "target: native\ncodex: {event: Stop, matcher: Bash, command: notify}\n", "native-matcher"},
		{"matcher on prompt", "target: native\nclaude-code: {event: UserPromptSubmit, matcher: Bash, command: notify}\n", "native-matcher"},
		{"bad regex", "target: native\ncodex: {event: PreToolUse, matcher: \"(\", command: notify}\n", "native-matcher"},
		{"codex star", "target: native\ncodex: {event: PreToolUse, matcher: \"*\", command: notify}\n", "native-matcher"},

		{"missing command", "target: native\ncodex: {event: Stop}\n", "native-command"},
		{"non-string command", "target: native\ncodex: {event: Stop, command: 3}\n", "native-command"},
		{"empty command", "target: native\ncodex: {event: Stop, command: \"\"}\n", "native-command"},
		{"padded command", "target: native\ncodex: {event: Stop, command: \" notify\"}\n", "native-command"},
		{"newline", "target: native\ncodex: {event: Stop, command: \"a\\nb\"}\n", "native-command"},
		{"carriage return", "target: native\ncodex: {event: Stop, command: \"a\\rb\"}\n", "native-command"},
		{"nul", "target: native\ncodex: {event: Stop, command: \"a\\0b\"}\n", "native-command"},
		{"leading semicolon", "target: native\ncodex: {event: Stop, command: \"; a\"}\n", "native-command"},
		{"leading ampersand", "target: native\ncodex: {event: Stop, command: \"& a\"}\n", "native-command"},
		{"leading pipe", "target: native\ncodex: {event: Stop, command: \"| a\"}\n", "native-command"},
		{"leading paren", "target: native\ncodex: {event: Stop, command: \") a\"}\n", "native-command"},
		{"leading comment", "target: native\ncodex: {event: Stop, command: \"# a\"}\n", "native-command"},
		{"trailing backslash", "target: native\ncodex: {event: Stop, command: \"notify \\\\\"}\n", "native-command"},
		{"open single quote", "target: native\ncodex: {event: Stop, command: \"echo 'a\"}\n", "native-command"},
		{"open double quote", "target: native\ncodex: {event: Stop, command: 'echo \"a'}\n", "native-command"},

		{"zero timeout", "target: native\ncodex: {event: Stop, command: a, timeout: 0}\n", "native-timeout"},
		{"large timeout", "target: native\ncodex: {event: Stop, command: a, timeout: 601}\n", "native-timeout"},
		{"fractional timeout", "target: native\ncodex: {event: Stop, command: a, timeout: 1.5}\n", "native-timeout"},
		{"string timeout", "target: native\ncodex: {event: Stop, command: a, timeout: \"10\"}\n", "native-timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results := map[string][]string{}
			for _, format := range []string{"directory", "single-file"} {
				codes := errorCodes(validateWithNative(loadNativeRule(t, format, tc.fields, ""), testNativeCatalog))
				if tc.want == "" && len(codes) != 0 {
					t.Fatalf("%s: unexpected errors %v", format, codes)
				}
				if tc.want != "" && !containsString(codes, tc.want) {
					t.Fatalf("%s: errors %v do not contain %s", format, codes, tc.want)
				}
				results[format] = codes
			}
			if !reflect.DeepEqual(results["directory"], results["single-file"]) {
				t.Fatalf("directory %v != single-file %v", results["directory"], results["single-file"])
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestHarnessKeyOnNonNativeRuleIsUnknownField(t *testing.T) {
	pkg := loadNativeRule(t, "directory", "target: system\ntext: style\ncodex: {event: Stop, command: notify}\n", "")
	codes := errorCodes(validateWithNative(pkg, testNativeCatalog))
	if !reflect.DeepEqual(codes, []string{"invalid-schema", "invalid-schema"}) {
		t.Fatalf("codes = %v, want unknown field and value type invalid-schema", codes)
	}
}

func TestNativeRulesIgnorePackageDefaultsAndAllowEmptyBodies(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "STYLE.yaml", testManifest("native-defaults", "defaults:\n  action: block\n  repeat: every\n  sources: [startup]\n", "  - from: rules/notify.md\n"))
	writeFile(t, root, "rules/notify.md", "---\ntarget: native\ncodex:\n  event: Stop\n  command: notify\n---\n")
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := validateWithNative(pkg, testNativeCatalog)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
}

func TestFromEntryReplacesWholeNativeMapping(t *testing.T) {
	for _, tc := range []struct {
		name, override, want string
	}{
		{"valid override", "    codex: {event: Stop, command: notify}\n", ""},
		{"invalid override", "    codex: {event: Stop}\n", "native-command"},
		{"engine field override", "    action: warn\n", "native-field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "STYLE.yaml", testManifest("native-from", "", "  - from: rules/notify.md\n"+tc.override))
			writeFile(t, root, "rules/notify.md", "---\ntarget: native\ncodex:\n  event: Stop\n---\nAuthor note.\n")
			pkg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			codes := errorCodes(validateWithNative(pkg, testNativeCatalog))
			if tc.want == "" && len(codes) != 0 {
				t.Fatalf("unexpected errors %v", codes)
			}
			if tc.want != "" && !containsString(codes, tc.want) {
				t.Fatalf("errors %v do not contain %s", codes, tc.want)
			}
		})
	}
}

func TestNativeHarnessKeysComeFromCatalog(t *testing.T) {
	catalog := NativeCatalog{"fake": fakeNativeHarness{events: map[string]bool{"Done": false}}}
	accepted := loadNativeRule(t, "directory", "target: native\nfake: {event: Done, command: notify}\n", "")
	if codes := errorCodes(validateWithNative(accepted, catalog)); len(codes) != 0 {
		t.Fatalf("registered fake harness rejected: %v", codes)
	}
	rejected := loadNativeRule(t, "directory", "target: native\ncodex: {event: Stop, command: notify}\n", "")
	diagnostics := ValidateNative(rejected, catalog)
	if codes := errorCodes(diagnostics); !reflect.DeepEqual(codes, []string{"native-field"}) {
		t.Fatalf("codes = %v, want native-field for an unregistered harness", codes)
	}
	if !strings.Contains(diagnostics[0].Message, "registered: fake") {
		t.Fatalf("message %q does not list registered harnesses", diagnostics[0].Message)
	}
}

func TestNativeRulesParsesMappings(t *testing.T) {
	pkg := loadNativeRule(t, "directory", "target: native\nclaude-code: {event: PreToolUse, matcher: \"^Bash$\", command: notify, timeout: 10}\ncodex: {event: Stop, command: \"notify --done\"}\n", "")
	got := NativeRules(pkg.Rules[0])
	want := []NativeRule{
		{RuleID: "native-rule", Harness: "claude-code", Event: "PreToolUse", Matcher: "^Bash$", Command: "notify", Timeout: 10},
		{RuleID: "native-rule", Harness: "codex", Event: "Stop", Command: "notify --done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeRules() = %#v, want %#v", got, want)
	}
}
