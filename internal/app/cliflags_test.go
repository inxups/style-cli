package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCommandHelpWorksInAnyPosition(t *testing.T) {
	service := testService(t)
	for _, section := range commandSections {
		for _, command := range section.Commands {
			want := command.usage() + "\n"
			if command.Name == "install" {
				want += frozenCommand.usage() + "\n"
			}
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
