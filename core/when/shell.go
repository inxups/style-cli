package when

import (
	"path/filepath"
	"slices"
	"strings"
)

// maxShellDepth bounds recursion through command substitutions, sh -c
// scripts, eval arguments, and heredocs fed to a shell.
const maxShellDepth = 8

// shellCommands splits a shell command string into the simple commands it
// would run, for the shell_regex operator. It is a best-effort POSIX-style
// reader, not a full shell: quotes are removed, comments, redirection
// targets and quoted heredoc bodies are data, and command substitutions,
// subshells, sh/bash -c scripts, eval arguments, heredocs and here-strings fed
// to a shell, echo/printf/cat output piped into a shell, the command run by a
// wrapper such as sudo or xargs, and find -exec commands are read as further
// commands. Malformed input never fails; an unterminated
// quote or substitution runs to the end of the input.
func shellCommands(script string) []string {
	var commands []string
	parseShell(script, 0, &commands)
	return commands
}

type shellCommand struct {
	words  []string
	bodies []string
	// stdin is the command piped into this one, if any.
	stdin *shellCommand
}

type shellHeredoc struct {
	command   *shellCommand
	delimiter string
	stripTabs bool
	quoted    bool
}

type shellReader struct {
	src   string
	pos   int
	depth int
	out   *[]string
}

func parseShell(src string, depth int, out *[]string) {
	if depth > maxShellDepth {
		return
	}
	r := &shellReader{src: src, depth: depth, out: out}
	var commands []*shellCommand
	var pending []shellHeredoc
	current := &shellCommand{}
	redirectTarget, hereString := false, false
	flush := func() {
		if len(current.words) > 0 || len(current.bodies) > 0 {
			commands = append(commands, current)
		}
		current = &shellCommand{}
		redirectTarget, hereString = false, false
	}
	for r.pos < len(src) {
		c := src[r.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			r.pos++
		case c == '\\' && r.peek(1) == '\n':
			r.pos += 2
		case c == '\n':
			r.pos++
			flush()
			for _, heredoc := range pending {
				r.readHeredoc(heredoc)
			}
			pending = nil
		case c == '#':
			for r.pos < len(src) && src[r.pos] != '\n' {
				r.pos++
			}
		case c == '&' && r.peek(1) == '>':
			r.pos += 2
			if r.peek(0) == '>' {
				r.pos++
			}
			redirectTarget = true
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			r.pos++
			pipe := c == '|'
			if next := r.peek(0); (c == ';' || c == '&' || c == '|') && (next == c || (c == '|' && next == '&')) {
				pipe = pipe && next == '&'
				r.pos++
			}
			source := current
			flush()
			if pipe && len(source.words) > 0 {
				current.stdin = source
			}
		case c == '<' && r.peek(1) == '<' && r.peek(2) == '<':
			r.pos += 3
			hereString = true
		case c == '<' && r.peek(1) == '<':
			r.pos += 2
			heredoc := shellHeredoc{command: current}
			if r.peek(0) == '-' {
				heredoc.stripTabs = true
				r.pos++
			}
			r.skipBlanks()
			heredoc.delimiter, heredoc.quoted = r.readWord()
			pending = append(pending, heredoc)
		case c == '<' || c == '>':
			r.pos++
			if next := r.peek(0); next == '>' || next == '|' || (c == '<' && next == '>') {
				r.pos++
			} else if next == '&' {
				r.pos++
				if d := r.peek(0); d == '-' || (d >= '0' && d <= '9') {
					for r.pos < len(src) && (src[r.pos] == '-' || (src[r.pos] >= '0' && src[r.pos] <= '9')) {
						r.pos++
					}
					continue
				}
			}
			redirectTarget = true
		default:
			start := r.pos
			word, _ := r.readWord()
			if redirectTarget {
				redirectTarget = false
				continue
			}
			if hereString {
				hereString = false
				current.bodies = append(current.bodies, word+"\n")
				continue
			}
			if next := r.peek(0); (next == '<' || next == '>') && isDigits(src[start:r.pos]) {
				continue
			}
			current.words = append(current.words, word)
		}
	}
	flush()
	for _, heredoc := range pending {
		r.readHeredoc(heredoc)
	}
	for _, command := range commands {
		r.finish(command)
	}
}

func (r *shellReader) peek(offset int) byte {
	if r.pos+offset < len(r.src) {
		return r.src[r.pos+offset]
	}
	return 0
}

func (r *shellReader) skipBlanks() {
	for r.pos < len(r.src) && (r.src[r.pos] == ' ' || r.src[r.pos] == '\t') {
		r.pos++
	}
}

func (r *shellReader) nested(script string) { parseShell(script, r.depth+1, r.out) }

// readWord reads one word, removing quotes. quoted reports whether any part
// of it was quoted or escaped, which makes a heredoc body literal.
func (r *shellReader) readWord() (string, bool) {
	var word strings.Builder
	quoted := false
	for r.pos < len(r.src) {
		c := r.src[r.pos]
		switch {
		case strings.IndexByte(" \t\r\n;&|()<>", c) >= 0:
			return word.String(), quoted
		case c == '\\':
			quoted = true
			if r.peek(1) == '\n' {
				r.pos += 2
			} else if r.pos+1 < len(r.src) {
				word.WriteByte(r.src[r.pos+1])
				r.pos += 2
			} else {
				r.pos++
			}
		case c == '\'':
			quoted = true
			end := strings.IndexByte(r.src[r.pos+1:], '\'')
			if end < 0 {
				word.WriteString(r.src[r.pos+1:])
				r.pos = len(r.src)
			} else {
				word.WriteString(r.src[r.pos+1 : r.pos+1+end])
				r.pos += end + 2
			}
		case c == '"':
			quoted = true
			r.pos++
			r.readDoubleQuoted(&word)
		case c == '$' && r.peek(1) == '\'':
			quoted = true
			r.pos += 2
			r.readANSIQuoted(&word)
		case c == '$' && r.peek(1) == '(':
			start := r.pos
			r.readSubstitution()
			word.WriteString(r.src[start:r.pos])
		case c == '$' && r.peek(1) == '{':
			start := r.pos
			r.pos = matchingBrace(r.src, r.pos+2)
			word.WriteString(r.src[start:r.pos])
		case c == '`':
			start := r.pos
			r.readBackquoted()
			word.WriteString(r.src[start:r.pos])
		default:
			word.WriteByte(c)
			r.pos++
		}
	}
	return word.String(), quoted
}

func (r *shellReader) readDoubleQuoted(word *strings.Builder) {
	for r.pos < len(r.src) {
		c := r.src[r.pos]
		switch {
		case c == '"':
			r.pos++
			return
		case c == '\\' && r.pos+1 < len(r.src) && strings.IndexByte("$`\"\\\n", r.src[r.pos+1]) >= 0:
			if r.src[r.pos+1] != '\n' {
				word.WriteByte(r.src[r.pos+1])
			}
			r.pos += 2
		case c == '$' && r.peek(1) == '(':
			start := r.pos
			r.readSubstitution()
			word.WriteString(r.src[start:r.pos])
		case c == '`':
			start := r.pos
			r.readBackquoted()
			word.WriteString(r.src[start:r.pos])
		default:
			word.WriteByte(c)
			r.pos++
		}
	}
}

func (r *shellReader) readANSIQuoted(word *strings.Builder) {
	escapes := map[byte]byte{'n': '\n', 't': '\t', 'r': '\r', '\\': '\\', '\'': '\'', '"': '"'}
	for r.pos < len(r.src) {
		c := r.src[r.pos]
		if c == '\'' {
			r.pos++
			return
		}
		if c == '\\' && r.pos+1 < len(r.src) {
			if value, ok := escapes[r.src[r.pos+1]]; ok {
				word.WriteByte(value)
			} else {
				word.WriteString(r.src[r.pos : r.pos+2])
			}
			r.pos += 2
			continue
		}
		word.WriteByte(c)
		r.pos++
	}
}

// readSubstitution reads $(...) starting at the dollar sign and parses its
// content as a nested script.
func (r *shellReader) readSubstitution() {
	contentEnd, next := matchingParen(r.src, r.pos+2)
	r.nested(r.src[r.pos+2 : contentEnd])
	r.pos = next
}

// readBackquoted reads `...` starting at the opening backquote and parses
// its content, with \` unescaped, as a nested script.
func (r *shellReader) readBackquoted() {
	var inner strings.Builder
	r.pos++
	for r.pos < len(r.src) {
		c := r.src[r.pos]
		if c == '`' {
			r.pos++
			break
		}
		if c == '\\' && r.pos+1 < len(r.src) && strings.IndexByte("`$\\", r.src[r.pos+1]) >= 0 {
			inner.WriteByte(r.src[r.pos+1])
			r.pos += 2
			continue
		}
		inner.WriteByte(c)
		r.pos++
	}
	r.nested(inner.String())
}

func (r *shellReader) readHeredoc(heredoc shellHeredoc) {
	var body strings.Builder
	for r.pos < len(r.src) {
		end := strings.IndexByte(r.src[r.pos:], '\n')
		line := r.src[r.pos:]
		if end >= 0 {
			line = r.src[r.pos : r.pos+end]
			r.pos += end + 1
		} else {
			r.pos = len(r.src)
		}
		check := line
		if heredoc.stripTabs {
			check = strings.TrimLeft(check, "\t")
		}
		if check == heredoc.delimiter {
			break
		}
		body.WriteString(line + "\n")
	}
	text := body.String()
	if !heredoc.quoted {
		r.substitutionsIn(text)
	}
	heredoc.command.bodies = append(heredoc.command.bodies, text)
}

// substitutionsIn parses the command substitutions of unquoted heredoc text,
// where quotes are literal but $(...) and `...` still run.
func (r *shellReader) substitutionsIn(text string) {
	inner := &shellReader{src: text, depth: r.depth, out: r.out}
	for inner.pos < len(text) {
		switch c := text[inner.pos]; {
		case c == '\\':
			inner.pos += 2
		case c == '$' && inner.peek(1) == '(':
			inner.readSubstitution()
		case c == '`':
			inner.readBackquoted()
		default:
			inner.pos++
		}
	}
}

func (r *shellReader) finish(command *shellCommand) {
	words := commandWords(command.words)
	if len(words) == 0 {
		return
	}
	*r.out = append(*r.out, renderShellWords(words))
	if inner := wrappedCommands(words); inner != nil {
		for _, wrapped := range inner {
			r.finish(&shellCommand{words: wrapped, bodies: command.bodies, stdin: command.stdin})
		}
		return
	}
	switch name := filepath.Base(words[0]); name {
	case "sh", "bash", "zsh", "dash", "ksh":
		if script, ok := shellScriptArgument(words[1:]); ok {
			r.nested(script)
			return
		}
		for _, body := range command.bodies {
			r.nested(body)
		}
		if command.stdin != nil && readsScriptFromStdin(words[1:]) {
			if script, ok := pipedScript(command.stdin); ok {
				r.nested(script)
			}
		}
	case "eval":
		r.nested(strings.Join(words[1:], " "))
	}
}

// commandWords drops leading reserved words, assignments, and wrappers given
// without options so that the command name comes first. A wrapper followed
// by an option stays in place for wrappedCommands.
func commandWords(words []string) []string {
	reserved := map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true, "do": true, "done": true,
		"while": true, "until": true, "!": true, "{": true, "}": true, "esac": true}
	wrappers := map[string]bool{"command": true, "builtin": true, "exec": true, "nohup": true, "env": true, "time": true}
	for len(words) > 0 {
		switch {
		case reserved[words[0]] || isAssignment(words[0]):
			words = words[1:]
		case wrappers[words[0]] && (len(words) == 1 || !strings.HasPrefix(words[1], "-")):
			words = words[1:]
		default:
			return words
		}
	}
	return words
}

// wrapperOptions lists, for each wrapper that runs the command after its
// options, the short options that take a value and the long options that
// take a separate one.
var wrapperOptions = map[string]struct {
	short string
	long  []string
}{
	"sudo":    {"ughpCDrtTUR", []string{"--user", "--group", "--host", "--prompt", "--close-from", "--chdir", "--role", "--type", "--command-timeout", "--other-user", "--chroot"}},
	"doas":    {"uC", nil},
	"env":     {"uCPS", []string{"--unset", "--chdir", "--split-string"}},
	"nice":    {"n", []string{"--adjustment"}},
	"timeout": {"sk", []string{"--signal", "--kill-after"}},
	"stdbuf":  {"ioe", []string{"--input", "--output", "--error"}},
	"xargs":   {"adEILnPs", []string{"--arg-file", "--delimiter", "--max-lines", "--max-args", "--max-procs", "--max-chars", "--process-slot-var"}},
	"time":    {"fo", []string{"--format", "--output"}},
	"exec":    {"a", nil},
	"command": {"", nil},
	"builtin": {"", nil},
	"nohup":   {"", nil},
}

// wrappedCommands returns the commands a wrapper runs: the words after the
// options of sudo, env, xargs and the like, or each find -exec command.
// The words of env -S are split on whitespace.
func wrappedCommands(words []string) [][]string {
	name := filepath.Base(words[0])
	if name == "find" {
		return findExecCommands(words[1:])
	}
	options, ok := wrapperOptions[name]
	if !ok {
		return nil
	}
	args, split := skipWrapperOptions(words[1:], options.short, options.long)
	if name == "timeout" && len(args) > 0 {
		args = args[1:]
	}
	args = append(strings.Fields(split), args...)
	if len(args) == 0 {
		return nil
	}
	return [][]string{args}
}

// skipWrapperOptions returns the arguments after a wrapper's options and the
// value of an env -S / --split-string option.
func skipWrapperOptions(args []string, short string, long []string) ([]string, string) {
	split := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return args[i+1:], split
		case arg == "-":
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg, "=")
			if !hasValue && slices.Contains(long, name) && i+1 < len(args) {
				i++
				value = args[i]
			}
			if name == "--split-string" {
				split = value
			}
		case arg[0] == '-':
			for j := 1; j < len(arg); j++ {
				if strings.IndexByte(short, arg[j]) < 0 {
					continue
				}
				value := arg[j+1:]
				if value == "" && i+1 < len(args) {
					i++
					value = args[i]
				}
				if arg[j] == 'S' {
					split = value
				}
				break
			}
		default:
			return args[i:], split
		}
	}
	return nil, split
}

// findExecCommands returns the commands of find -exec, -execdir, -ok and
// -okdir, each ending at ; or +.
func findExecCommands(args []string) [][]string {
	var commands [][]string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-exec", "-execdir", "-ok", "-okdir":
			end := i + 1
			for end < len(args) && args[end] != ";" && args[end] != "+" {
				end++
			}
			if end > i+1 {
				commands = append(commands, args[i+1:end])
			}
			i = end
		}
	}
	return commands
}

// readsScriptFromStdin reports whether a shell given these arguments reads
// its script from standard input: no script file operand, or -s.
func readsScriptFromStdin(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return i+1 == len(args)
		case arg == "-o" || arg == "+o" || arg == "--rcfile" || arg == "--init-file":
			i++
		case len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.IndexByte(arg[1:], 's') >= 0:
			return true
		case len(arg) > 1 && (arg[0] == '-' || arg[0] == '+'):
		default:
			return false
		}
	}
	return true
}

// pipedScript returns what echo, printf, or cat with only heredoc or
// here-string input writes, which a shell reading standard input then runs.
func pipedScript(source *shellCommand) (string, bool) {
	words := commandWords(source.words)
	if len(words) == 0 {
		return "", false
	}
	args := words[1:]
	switch filepath.Base(words[0]) {
	case "echo":
		for len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' && strings.Trim(args[0][1:], "neE") == "" {
			args = args[1:]
		}
		return strings.Join(args, " "), true
	case "printf":
		if len(args) == 0 {
			return "", false
		}
		return printfText(args[0], args[1:]), true
	case "cat":
		if len(source.bodies) > 0 && (len(args) == 0 || len(args) == 1 && args[0] == "-") {
			return strings.Join(source.bodies, ""), true
		}
	}
	return "", false
}

// printfText expands the %s, %% and backslash escapes of a printf format
// once; other directives are left as written.
func printfText(format string, args []string) string {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if i+1 == len(format) || (c != '%' && c != '\\') {
			out.WriteByte(c)
			continue
		}
		i++
		switch next := format[i]; {
		case c == '%' && next == 's':
			if len(args) > 0 {
				out.WriteString(args[0])
				args = args[1:]
			}
		case c == '%' && next == '%':
			out.WriteByte('%')
		case c == '\\' && next == 'n':
			out.WriteByte('\n')
		case c == '\\' && next == 't':
			out.WriteByte('\t')
		case c == '\\' && next == '\\':
			out.WriteByte('\\')
		default:
			out.WriteByte(c)
			out.WriteByte(next)
		}
	}
	return out.String()
}

// shellScriptArgument returns the script of `sh -c script` style options,
// including combined flags such as -lc and -ec.
func shellScriptArgument(args []string) (string, bool) {
	command := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || len(arg) < 2 || (arg[0] != '-' && arg[0] != '+') {
			if command {
				if arg == "--" && i+1 < len(args) {
					return args[i+1], true
				}
				return arg, arg != "--"
			}
			return "", false
		}
		if arg == "-o" || arg == "+o" {
			i++
			continue
		}
		if arg[0] == '-' && arg[1] != '-' && strings.IndexByte(arg[1:], 'c') >= 0 {
			command = true
		}
	}
	return "", false
}

// renderShellWords joins words with single spaces. Words with characters
// outside a conservative safe set are single-quoted, so whitespace inside one
// argument cannot look like an argument boundary to a pattern.
func renderShellWords(words []string) string {
	rendered := make([]string, len(words))
	for i, word := range words {
		rendered[i] = word
		if word == "" || strings.IndexFunc(word, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-~^", r))
		}) >= 0 {
			rendered[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
		}
	}
	return strings.Join(rendered, " ")
}

func isAssignment(word string) bool {
	equals := strings.IndexByte(word, '=')
	if equals <= 0 {
		return false
	}
	for i := 0; i < equals; i++ {
		c := word[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

// matchingParen finds the ) closing a $( whose content starts at start,
// skipping quoted text. It returns the end of the content and the index just
// past the parenthesis; both are len(src) when it is unterminated.
func matchingParen(src string, start int) (int, int) {
	depth := 1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '\'':
			end := strings.IndexByte(src[i+1:], '\'')
			if end < 0 {
				return len(src), len(src)
			}
			i += end + 1
		case '"':
			for i++; i < len(src) && src[i] != '"'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, i + 1
			}
		}
	}
	return len(src), len(src)
}

func matchingBrace(src string, start int) int {
	depth := 1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(src)
}
