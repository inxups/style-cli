package spec

import (
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
	return ValidateWithCatalog(pkg, ValidationCatalog{Harnesses: catalog.Names(), Native: catalog})
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

// loadNativeHook loads the canonical inline declaration from disk or a snapshot.
func loadNativeHook(t *testing.T, format, fields, description string) *Package {
	t.Helper()
	root := t.TempDir()
	if description != "" {
		fields += "description: " + description + "\n"
	}
	indented := strings.ReplaceAll(strings.TrimSuffix(fields, "\n"), "\n", "\n    ")
	manifest := "spec: style/v0.3\nname: native-rule\nversion: 1.0.0\ndescription: Native hook\nnative_hooks:\n  - id: native-rule\n    " + indented + "\n"
	if format == "snapshot" {
		pkg, err := LoadSnapshot(root, map[string][]byte{"STYLE.yaml": []byte(manifest)}, "")
		if err != nil {
			t.Fatal(err)
		}
		return pkg
	}
	writeFile(t, root, "STYLE.yaml", manifest)
	pkg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestNativeHookValidationMatchesDiskAndSnapshot(t *testing.T) {
	const stop = "codex: {event: Stop, command: notify}\n"
	cases := []struct {
		name, fields, want string // want is the required error code; "" means valid
	}{
		{"codex only", stop, ""},
		{"claude only with matcher and timeout", "claude-code: {event: PostToolUse, matcher: \"^Bash$\", command: \"notify --title claude\", timeout: 10}\n", ""},
		{"both harnesses", stop + "claude-code: {event: Stop, command: notify}\n", ""},
		{"claude star matcher", "claude-code: {event: PreToolUse, matcher: \"*\", command: notify}\n", ""},
		{"quoted command", "codex: {event: Stop, command: 'printf \"%s\" ''a b'' | tee -a log && echo \\;'}\n", ""},
		{"timeout bounds", "codex: {event: Stop, command: a, timeout: 1}\nclaude-code: {event: Stop, command: a, timeout: 600}\n", ""},
		{"disabled hook still validated", "enabled: false\ncodex: {event: Stop}\n", "native-command"},

		{"no harness", "", "native-mapping"},
		{"list mapping", "codex: [Stop]\n", "native-mapping"},
		{"scalar mapping", "codex: notify\n", "native-mapping"},
		{"null mapping", "codex:\n", "native-mapping"},

		{"action", "action: warn\n" + stop, "native-field"},
		{"when", "when: {harness: {name: codex}}\n" + stop, "native-field"},
		{"repeat", "repeat: every\n" + stop, "native-field"},
		{"event field", "event: tool.pre\n" + stop, "native-field"},
		{"unregistered harness", "gemini: {event: Stop, command: notify}\n", "native-field"},
		{"async", "codex: {event: Stop, command: notify, async: true}\n", "native-field"},
		{"onFailure", "claude-code: {event: PreToolUse, command: notify, onFailure: block}\n", "native-field"},
		{"type", "codex: {event: Stop, command: notify, type: command}\n", "native-field"},
		{"args", "claude-code: {event: Stop, command: notify, args: [a]}\n", "native-field"},

		{"missing event", "codex: {command: notify}\n", "native-event"},
		{"non-string event", "codex: {event: 3, command: notify}\n", "native-event"},
		{"codex unknown event", "codex: {event: UserPromptExpansion, command: notify}\n", "native-event"},
		{"claude unknown event", "claude-code: {event: PostModelSwitch, command: notify}\n", "native-event"},

		{"empty matcher", "codex: {event: PreToolUse, matcher: \"\", command: notify}\n", "native-matcher"},
		{"padded matcher", "codex: {event: PreToolUse, matcher: \" Bash\", command: notify}\n", "native-matcher"},
		{"matcher on Stop", "codex: {event: Stop, matcher: Bash, command: notify}\n", "native-matcher"},
		{"matcher on prompt", "claude-code: {event: UserPromptSubmit, matcher: Bash, command: notify}\n", "native-matcher"},
		{"bad regex", "codex: {event: PreToolUse, matcher: \"(\", command: notify}\n", "native-matcher"},
		{"codex star", "codex: {event: PreToolUse, matcher: \"*\", command: notify}\n", "native-matcher"},

		{"missing command", "codex: {event: Stop}\n", "native-command"},
		{"non-string command", "codex: {event: Stop, command: 3}\n", "native-command"},
		{"empty command", "codex: {event: Stop, command: \"\"}\n", "native-command"},
		{"padded command", "codex: {event: Stop, command: \" notify\"}\n", "native-command"},
		{"newline", "codex: {event: Stop, command: \"a\\nb\"}\n", "native-command"},
		{"carriage return", "codex: {event: Stop, command: \"a\\rb\"}\n", "native-command"},
		{"nul", "codex: {event: Stop, command: \"a\\0b\"}\n", "native-command"},
		{"leading semicolon", "codex: {event: Stop, command: \"; a\"}\n", "native-command"},
		{"leading ampersand", "codex: {event: Stop, command: \"& a\"}\n", "native-command"},
		{"leading pipe", "codex: {event: Stop, command: \"| a\"}\n", "native-command"},
		{"leading paren", "codex: {event: Stop, command: \") a\"}\n", "native-command"},
		{"leading comment", "codex: {event: Stop, command: \"# a\"}\n", "native-command"},
		{"trailing backslash", "codex: {event: Stop, command: \"notify \\\\\"}\n", "native-command"},
		{"open single quote", "codex: {event: Stop, command: \"echo 'a\"}\n", "native-command"},
		{"open double quote", "codex: {event: Stop, command: 'echo \"a'}\n", "native-command"},

		{"zero timeout", "codex: {event: Stop, command: a, timeout: 0}\n", "native-timeout"},
		{"large timeout", "codex: {event: Stop, command: a, timeout: 601}\n", "native-timeout"},
		{"fractional timeout", "codex: {event: Stop, command: a, timeout: 1.5}\n", "native-timeout"},
		{"string timeout", "codex: {event: Stop, command: a, timeout: \"10\"}\n", "native-timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results := map[string][]string{}
			for _, format := range []string{"directory", "snapshot"} {
				codes := errorCodes(validateWithNative(loadNativeHook(t, format, tc.fields, ""), testNativeCatalog))
				if tc.want == "" && len(codes) != 0 {
					t.Fatalf("%s: unexpected errors %v", format, codes)
				}
				if tc.want != "" && !containsString(codes, tc.want) {
					t.Fatalf("%s: errors %v do not contain %s", format, codes, tc.want)
				}
				results[format] = codes
			}
			if !reflect.DeepEqual(results["directory"], results["snapshot"]) {
				t.Fatalf("directory %v != snapshot %v", results["directory"], results["snapshot"])
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

func TestNativeHarnessKeysComeFromCatalog(t *testing.T) {
	catalog := NativeCatalog{"fake": fakeNativeHarness{events: map[string]bool{"Done": false}}}
	accepted := loadNativeHook(t, "directory", "fake: {event: Done, command: notify}\n", "")
	if codes := errorCodes(validateWithNative(accepted, catalog)); len(codes) != 0 {
		t.Fatalf("registered fake harness rejected: %v", codes)
	}
	rejected := loadNativeHook(t, "directory", "codex: {event: Stop, command: notify}\n", "")
	diagnostics := ValidateNative(rejected, catalog)
	if codes := errorCodes(diagnostics); !reflect.DeepEqual(codes, []string{"native-field"}) {
		t.Fatalf("codes = %v, want native-field for an unregistered harness", codes)
	}
	if !strings.Contains(diagnostics[0].Message, "registered: fake") {
		t.Fatalf("message %q does not list registered harnesses", diagnostics[0].Message)
	}
}

func TestNativeMappingsParseHookDeclarations(t *testing.T) {
	pkg := loadNativeHook(t, "directory", "claude-code: {event: PreToolUse, matcher: \"^Bash$\", command: notify, timeout: 10}\ncodex: {event: Stop, command: \"notify --done\"}\n", "")
	got := NativeMappings(pkg.NativeHooks[0])
	want := []NativeMapping{
		{HookID: "native-rule", Harness: "claude-code", Event: "PreToolUse", Matcher: "^Bash$", Command: "notify", Timeout: 10},
		{HookID: "native-rule", Harness: "codex", Event: "Stop", Command: "notify --done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeMappings() = %#v, want %#v", got, want)
	}
}
