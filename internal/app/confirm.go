package app

import (
	"bufio"
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
		line, _ = bufio.NewReader(io.LimitReader(in, 256)).ReadString('\n')
	}
	// A terminal echoes the typed answer; piped answers are not echoed.
	if !isTerminal(in) || !strings.HasSuffix(line, "\n") {
		fmt.Fprintln(errOut)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func isTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
