package when

import (
	"reflect"
	"strings"
	"testing"
)

func TestShellCommandsSplitsSimpleCommands(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   []string
	}{
		{"plain", "git push origin main", []string{"git push origin main"}},
		{"separators", "a; b && c || d | e & f |& g", []string{"a", "b", "c", "d", "e", "f", "g"}},
		{"newline and continuation", "cd repo\ngit push \\\n  --force", []string{"cd repo", "git push --force"}},
		{"quote removal keeps argument boundaries", `git commit -m "fix: a b" 'it''s' +main "+x"`, []string{`git commit -m 'fix: a b' its +main +x`}},
		{"quote inside quoted word", `echo "it's"`, []string{`echo 'it'\''s'`}},
		{"ansi c quote", `printf $'a\tb'`, []string{"printf 'a\tb'"}},
		{"comment", "echo hi # git push --force\n# git push -f", []string{"echo hi"}},
		{"hash inside word", "echo a#b", []string{"echo 'a#b'"}},
		{"redirections are not arguments", "git push -f 2>&1 >out.log < in &>all | tee log", []string{"git push -f", "tee log"}},
		{"subshell", "(cd repo && git push --force)", []string{"cd repo", "git push --force"}},
		{"command substitution", `out=$(git push --force) echo "$(git status)"`, []string{"git push --force", "git status", `echo '$(git status)'`}},
		{"nested substitution", "echo $(a $(b))", []string{"b", "a '$(b)'", "echo '$(a $(b))'"}},
		{"backquotes", "out=`git push -f`", []string{"git push -f"}},
		{"assignments and wrappers", "FOO=1 env BAR=2 command nohup git push -f", []string{"git push -f"}},
		{"reserved words", "if true; then git push -f; fi", []string{"true", "git push -f"}},
		{"sh -c", `bash -lc "cd repo && git push --force"`, []string{`bash -lc 'cd repo && git push --force'`, "cd repo", "git push --force"}},
		{"sh -c after options", `/bin/sh -e -o pipefail -c 'git push -f' name`, []string{`/bin/sh -e -o pipefail -c 'git push -f' name`, "git push -f"}},
		{"eval", `eval "git push -f"`, []string{`eval 'git push -f'`, "git push -f"}},
		{"quoted heredoc is data", "python3 - <<'PY'\nprint(\"$(git push -f)\")\ngit push --force\nPY\necho done", []string{"python3 -", "echo done"}},
		{"unquoted heredoc substitutions run", "cat <<EOF\ngit push --force\n$(git push -f)\nEOF", []string{"git push -f", "cat"}},
		{"heredoc fed to a shell", "bash <<-'EOF'\n\tgit push --force\n\tEOF\necho done", []string{"bash", "git push --force", "echo done"}},
		{"here string is data", "cat <<< 'git push -f'", []string{"cat"}},
		{"unterminated quote", `echo "git push -f`, []string{`echo 'git push -f'`}},
		{"unterminated substitution", "echo $(git push -f", []string{"git push -f", "echo '$(git push -f'"}},
		{"parameter expansion", "echo ${HOME:-/x y}", []string{"echo '${HOME:-/x y}'"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellCommands(tc.script); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("shellCommands(%q)\n got %q\nwant %q", tc.script, got, tc.want)
			}
		})
	}
}

func TestShellCommandsBoundsNesting(t *testing.T) {
	script := "git push -f"
	for i := 0; i < maxShellDepth+3; i++ {
		script = "bash -c " + renderShellWords([]string{script})
	}
	for _, command := range shellCommands(script) {
		if command == "git push -f" {
			t.Fatal("nesting beyond maxShellDepth must not be read")
		}
	}
	if got := shellCommands(strings.Repeat("$(", 10000)); len(got) == 0 {
		t.Fatal("deep unterminated substitutions must still yield the outer commands")
	}
}

func TestShellRegexMatchesAnySimpleCommand(t *testing.T) {
	rule := Rule{Agent: []string{"*"}, Conditions: []Condition{{Field: "tool.input.command", Op: "shell_regex", Pattern: `^git\s+push\b.*\s--force(?:\s|$)`}}}
	for script, want := range map[string]bool{
		"cd repo && git push --force": true,
		"echo 'git push --force'":     false,
		"git push origin 'a --force'": false,
		"git push --force-with-lease": false,
	} {
		if got := Match(rule, Input{Fields: FieldMap{"tool.input.command": script}}); got != want {
			t.Errorf("shell_regex on %q = %v, want %v", script, got, want)
		}
	}
	if _, err := Compile(Rule{Conditions: []Condition{{Field: "prompt", Op: "shell_regex", Pattern: "["}}}); err == nil {
		t.Fatal("shell_regex must reject an invalid regular expression")
	}
}

func FuzzShellCommands(f *testing.F) {
	for _, seed := range []string{"a; b", "x=$(y `z`)", "cat <<-'E'\n\tq\n\tE", "bash -c \"$(", "${a", "'", "\\", "2>&1 <<<x"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, script string) {
		shellCommands(script)
	})
}
