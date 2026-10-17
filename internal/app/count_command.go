package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/inxups/style-cli/internal/saferead"
	"github.com/inxups/style-cli/internal/tokencount"
)

const countFlags = "[--count=estimate|exact] [--count-provider=openai|anthropic] [--encoding=name] [--count-model=id]"
const countUsage = "style count <UTF-8-file> " + countFlags + " [--json]"

func parseCountOption(args []string, i *int, o *tokencount.Options) (bool, error) {
	name, value, inline := strings.Cut(args[*i], "=")
	var dst *string
	switch name {
	case "--count":
		dst = &o.Mode
	case "--count-provider":
		dst = &o.Provider
	case "--encoding":
		dst = &o.Encoding
	case "--count-model":
		dst = &o.Model
	default:
		return false, nil
	}
	if *dst != "" {
		return true, fmt.Errorf("duplicate option %s", name)
	}
	if !inline {
		*i += 1
		if *i >= len(args) || strings.HasPrefix(args[*i], "-") {
			return true, fmt.Errorf("missing value for %s", name)
		}
		value = args[*i]
	}
	if value == "" {
		return true, fmt.Errorf("empty option %s", name)
	}
	*dst = value
	return true, nil
}
func runCount(ctx context.Context, s *Service, workDir string, args []string, out, errOut io.Writer) int {
	if isHelp(args) {
		fmt.Fprintln(out, countUsage)
		return ExitOK
	}
	var options tokencount.Options
	path := ""
	jsonMode := false
	fail := func(err error) int { fmt.Fprintln(errOut, "style count:", err); return ExitInvalid }
	for i := 0; i < len(args); i++ {
		if handled, err := parseCountOption(args, &i, &options); handled {
			if err != nil {
				return fail(err)
			}
			continue
		}
		if args[i] == "--json" {
			if jsonMode {
				return fail(fmt.Errorf("duplicate --json"))
			}
			jsonMode = true
			continue
		}
		if strings.HasPrefix(args[i], "-") || path != "" {
			return fail(fmt.Errorf("expected one file and valid count options"))
		}
		path = args[i]
	}
	if path == "" {
		return fail(fmt.Errorf("UTF-8 file is required"))
	}
	if err := options.Validate(); err != nil {
		return fail(err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	data, err := saferead.ReadFile(path, tokencount.MaxTextBytes)
	if err != nil || !utf8.Valid(data) {
		return fail(fmt.Errorf("cannot read bounded regular UTF-8 file"))
	}
	result, err := s.countText(ctx, string(data), options)
	if err != nil {
		return fail(err)
	}
	if jsonMode {
		err = encodeJSON(out, struct {
			Format  int               `json:"format"`
			Command string            `json:"command"`
			Count   tokencount.Result `json:"count"`
		}{1, "count", result})
	} else {
		writeCountText(out, result)
	}
	if err != nil {
		return fail(fmt.Errorf("cannot write count report"))
	}
	return ExitOK
}
func writeCountText(out io.Writer, r tokencount.Result) {
	fmt.Fprintf(out, "%d tokens; mode=%s provider=%s scope=%s encoding=%s model=%s\ntext_sha256=%s implementation=%s api_version=%s\n", r.Tokens, r.Mode, r.Provider, r.Scope, r.Encoding, r.Model, r.TextSHA256, r.Implementation, r.APIVersion)
}
