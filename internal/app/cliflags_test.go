package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

// approve answers one confirmation prompt with yes.
func approve() io.Reader { return strings.NewReader("y\n") }

func TestCommandHelpWorksInAnyPosition(t *testing.T) {
	service := testService(t)
	for _, section := range commandSections {
		for _, command := range section.Commands {
			want := command.usage() + "\n"
			for _, args := range [][]string{
				{command.Name, "--help"},
				{command.Name, "operand", "-h"},
				{command.Name, "--unknown", "--help", "--json"},
				{command.Name, "--help", "--project", t.TempDir()},
				{"help", command.Name},
			} {
				var out, errOut bytes.Buffer
				if code := RunWithService(context.Background(), service, t.TempDir(), args, &out, &errOut); code != ExitOK || out.String() != want || errOut.Len() != 0 {
					t.Fatalf("%v: exit=%d stdout=%q stderr=%q want %q", args, code, out.String(), errOut.String(), want)
				}
			}
		}
	}
	for _, args := range [][]string{{"help", "nope"}, {"help", "list", "extra"}} {
		var out, errOut bytes.Buffer
		if code := RunWithService(context.Background(), service, t.TempDir(), args, &out, &errOut); code != ExitUsage || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out.String(), errOut.String())
		}
	}
}

func TestCommandSpecParsing(t *testing.T) {
	spec := commandSpec{Name: "demo", Args: "[name]", MaxArgs: 1, Flags: []flagSpec{
		jsonFlag, refFlag,
		{Name: "--set", Kind: flagList, Value: "k=v"},
		{Name: "--need", Kind: flagString, Value: "x", Required: true},
	}}
	parsed, err := spec.parse([]string{"pkg", "--need=a", "--ref", "main", "--set", "a=1", "--set=b=2", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Arg(0) != "pkg" || parsed.String("--need") != "a" || parsed.String("--ref") != "main" || !parsed.Bool("--json") || strings.Join(parsed.List("--set"), ",") != "a=1,b=2" {
		t.Fatalf("parsed = %+v", parsed)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--need=a", "one", "two"}, `unexpected argument "two"`},
		{[]string{"--need=a", "--other"}, `unknown option "--other"`},
		{[]string{"--need=a", "--need=b"}, "duplicate option --need"},
		{[]string{"--need=a", "--json", "--json"}, "duplicate option --json"},
		{[]string{"--need=a", "--json=true"}, "--json does not take a value"},
		{[]string{"--need=a", "--ref"}, "missing value for --ref"},
		{[]string{"--need=a", "--ref", "--json"}, "missing value for --ref"},
		{[]string{"--need=a", "--ref="}, "empty value for --ref"},
		{[]string{"pkg"}, "missing required option --need"},
	} {
		if _, err := spec.parse(test.args); err == nil || err.Error() != test.want {
			t.Fatalf("%v: err=%v want %q", test.args, err, test.want)
		}
	}
	// A negative number is a value for the flag to check, not a missing one.
	if parsed, err := spec.parse([]string{"--need", "-1h"}); err != nil || parsed.String("--need") != "-1h" {
		t.Fatalf("negative value: parsed=%+v err=%v", parsed, err)
	}
	if got, want := spec.usage(), "style demo [name] [--json] [--ref ref] [--set k=v]... --need x"; got != want {
		t.Fatalf("usage = %q want %q", got, want)
	}
}

func TestTopLevelHelpListsEveryCommandSummary(t *testing.T) {
	var out bytes.Buffer
	printUsage(&out)
	for _, section := range commandSections {
		for _, command := range section.Commands {
			if !strings.Contains(out.String(), "  "+command.summary()+"\n") {
				t.Fatalf("top-level help misses %s: %s", command.Name, &out)
			}
		}
	}
}

func TestInstallFailClosedIsClaudeCodeBoolean(t *testing.T) {
	request, _, err := parseInstallArgs([]string{"pkg", "--fail-closed"})
	if err != nil || strings.Join(request.FailClosed, ",") != "claude-code" {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	if request, _, err = parseInstallArgs([]string{"pkg"}); err != nil || request.FailClosed != nil {
		t.Fatalf("default request=%+v err=%v", request, err)
	}
	for _, args := range [][]string{{"pkg", "--fail-closed=claude-code"}, {"pkg", "--fail-closed", "claude-code"}, {"pkg", "--fail-closed", "--fail-closed"}} {
		if _, _, err := parseInstallArgs(args); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}

func TestExactCountOptionNamesProviderExplicitly(t *testing.T) {
	for _, test := range []struct {
		value                     string
		provider, encoding, model string
	}{{"openai:o200k_base", "openai", "o200k_base", ""}, {"anthropic:claude-opus-5-5", "anthropic", "", "claude-opus-5-5"}} {
		options, err := countOptions(parsedArgs{values: map[string][]string{"--exact": {test.value}}})
		if err != nil || options.Mode != "exact" || options.Provider != test.provider || options.Encoding != test.encoding || options.Model != test.model || options.Validate() != nil {
			t.Fatalf("%s: options=%+v err=%v", test.value, options, err)
		}
	}
	if options, err := countOptions(parsedArgs{values: map[string][]string{}}); err != nil || options.Exact() {
		t.Fatalf("default must estimate: %+v %v", options, err)
	}
	for _, value := range []string{"openai", "openai:", ":o200k_base", "claude-opus-5-5", "gemini:x"} {
		if _, err := countOptions(parsedArgs{values: map[string][]string{"--exact": {value}}}); err == nil {
			t.Fatalf("%s accepted", value)
		}
	}
}

func TestConfirmInputRequiresExplicitYes(t *testing.T) {
	for input, want := range map[string]bool{"y\n": true, "YES\n": true, " y \n": true, "y": true, "": false, "\n": false, "n\n": false, "yep\n": false} {
		var prompt bytes.Buffer
		if got := confirmInput(strings.NewReader(input), &prompt, "Apply?"); got != want || prompt.String() != "Apply? [y/N] \n" {
			t.Fatalf("%q: got %t prompt %q", input, got, prompt.String())
		}
	}
	var prompt bytes.Buffer
	if confirmInput(nil, &prompt, "Apply?") || prompt.String() != "Apply? [y/N] \n" {
		t.Fatalf("nil input approved or prompt %q", prompt.String())
	}
}
