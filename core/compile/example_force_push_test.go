package compile

import (
	"testing"

	"github.com/inxups/style-cli/core/when"
)

// M1B-EXAMPLE-01/02: the example block rule must treat only commands the shell
// would run as git invocations. Quoted text such as heredoc bodies or commit
// messages that merely mention a force push must not be blocked.
func TestExampleNoForcePushMatchesCommandPositionsOnly(t *testing.T) {
	result, _ := buildExampleIndex(t)
	var rule *when.Rule
	for _, entry := range result.Index.Harnesses["codex"]["tool.pre"] {
		if entry.Rule == "terse-reviewer/no-force-push" {
			rule = &entry.When
			break
		}
	}
	if rule == nil {
		t.Fatal("terse-reviewer/no-force-push@tool.pre not compiled")
	}
	matcher, err := when.Compile(*rule)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		command string
		blocked bool
	}{
		{"long flag", "git push --force", true},
		{"short flag", "git push -f origin main", true},
		{"plus refspec", "git push origin +main", true},
		{"dash C", "git -C repo push --force origin main", true},
		{"after and", "cd repo && git push --force", true},
		{"after semicolon", "cd repo; git push --force", true},
		{"after or", "false || git push --force origin main", true},
		{"next line", "cd repo\ngit push --force origin main", true},
		{"subshell", "(cd repo && git push --force)", true},
		{"command substitution", "out=$(git push --force)", true},
		{"backtick", "out=`git push --force`", true},
		{"bash -c", "bash -lc \"cd repo && git push --force\"", true},
		{"eval", "eval 'git push -f origin main'", true},
		{"heredoc fed to bash", "bash <<'EOF'\ngit push --force\nEOF", true},
		{"substitution in unquoted heredoc", "cat <<EOF\n$(git push -f)\nEOF", true},
		{"assignment and redirection", "GIT_TRACE=1 git push --force 2>&1 | tee push.log", true},
		{"line continuation", "git push \\\n  --force origin main", true},
		{"quoted plus refspec", "git push origin \"+main\"", true},
		{"lease", "git push --force-with-lease origin main", false},
		{"heredoc line starting with git", "cat > notes.md <<'EOF'\ngit push --force origin main\nEOF", false},
		{"comment", "git status # never git push --force", false},
		{"flag inside one argument", "git push origin 'topic --force'", false},
		{"echo of the command", "echo git push --force", false},
		{"plain push", "git push origin main", false},
		{"python heredoc text", "python3 - <<'PY'\ntext = \"git push --force origin main\"\nPY", false},
		{"single quoted text", "printf '%s\\n' 'git push --force origin main' >> notes.md", false},
		{"double quoted text", "echo \"use git push -f only with approval\"", false},
		{"commit message", "git commit -m \"avoid git push --force on main\"", false},
		{"quoted after plain push", "git push origin main && echo 'git push --force origin main'", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := when.Input{AgentID: "main", Fields: when.FieldMap{"tool.input.command": tc.command}}
			if got := matcher.Match(input); got != tc.blocked {
				t.Fatalf("Match(%q) = %v, want %v", tc.command, got, tc.blocked)
			}
		})
	}
}
