package when

import (
	"path/filepath"
	"strings"
)

// maxShellDepth bounds recursion through command substitutions, sh -c
// scripts, eval arguments, and heredocs fed to a shell.
const maxShellDepth = 8

// shellCommands splits a shell command string into the simple commands it
// would run, for the shell_regex operator. It is a best-effort POSIX-style
// reader, not a full shell: quotes are removed, comments, redirection
// targets and quoted heredoc bodies are data, and command substitutions,
// subshells, sh/bash -c scripts, eval arguments and heredocs fed to a shell
// are read as further commands. Malformed input never fails; an unterminated
// quote or substitution runs to the end of the input.
func shellCommands(script string) []string {
	var commands []string
	parseShell(script, 0, &commands)
	return commands
}

type shellCommand struct {
	words  []string
	bodies []string
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
	redirectTarget := false
	flush := func() {
		if len(current.words) > 0 || len(current.bodies) > 0 {
			commands = append(commands, current)
		}
		current = &shellCommand{}
		redirectTarget = false
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
			if next := r.peek(0); (c == ';' || c == '&' || c == '|') && (next == c || (c == '|' && next == '&')) {
				r.pos++
			}
			flush()
		case c == '<' && r.peek(1) == '<' && r.peek(2) == '<':
			r.pos += 3
			redirectTarget = true
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
	switch name := filepath.Base(words[0]); name {
	case "sh", "bash", "zsh", "dash", "ksh":
		if script, ok := shellScriptArgument(words[1:]); ok {
			r.nested(script)
			return
		}
		for _, body := range command.bodies {
			r.nested(body)
		}
	case "eval":
		r.nested(strings.Join(words[1:], " "))
	}
}

// commandWords drops leading reserved words, assignments, and simple
// wrappers so that the command name comes first. Wrappers that take option
// arguments, such as sudo, are left in place.
func commandWords(words []string) []string {
	reserved := map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true, "do": true, "done": true,
		"while": true, "until": true, "!": true, "{": true, "}": true, "time": true, "esac": true}
	wrappers := map[string]bool{"command": true, "builtin": true, "exec": true, "nohup": true, "env": true}
	for len(words) > 0 && (reserved[words[0]] || wrappers[words[0]] || isAssignment(words[0])) {
		words = words[1:]
	}
	return words
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
