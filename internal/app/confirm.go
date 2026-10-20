package app

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// confirmInput asks on errOut and reads one answer line from in. Only y or yes
// approves; no input, EOF or any other answer declines, so pipes and CI need
// an explicit answer (for example `echo y | style install ...`).
func confirmInput(in io.Reader, errOut io.Writer, question string) bool {
	fmt.Fprintf(errOut, "%s [y/N] ", question)
	line := ""
	if in != nil {
		line = readLine(in, 256)
	}
	// A terminal echoes the typed answer; piped answers are not echoed.
	if !isTerminal(in) || !strings.HasSuffix(line, "\n") {
		fmt.Fprintln(errOut)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// readLine reads up to limit bytes through the first newline, one byte at a
// time, so a later prompt still gets the next piped answer.
func readLine(in io.Reader, limit int) string {
	var line []byte
	buffer := make([]byte, 1)
	for len(line) < limit {
		if n, _ := in.Read(buffer); n == 0 {
			break
		}
		line = append(line, buffer[0])
		if buffer[0] == '\n' {
			break
		}
	}
	return string(line)
}

func isTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// declinedDiagnostic explains a preview that was not confirmed. In --json the
// preview stays the result; nothing was changed either way.
func declinedDiagnostic(command string) Diagnostic {
	return Diagnostic{Code: "confirmation-declined", Severity: "error", Message: command + " canceled; nothing was changed"}
}
