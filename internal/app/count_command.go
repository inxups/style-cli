package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"unicode/utf8"

	"github.com/inxups/style-cli/internal/saferead"
	"github.com/inxups/style-cli/internal/tokencount"
)

func countOptions(parsed parsedArgs) tokencount.Options {
	return tokencount.Options{Mode: parsed.String("--count"), Provider: parsed.String("--count-provider"), Encoding: parsed.String("--encoding"), Model: parsed.String("--count-model")}
}
func runCount(ctx context.Context, s *Service, workDir string, args []string, out, errOut io.Writer) int {
	parsed, err := countCommand.parse(args)
	if err != nil {
		return countCommand.usageError(errOut, err)
	}
	fail := func(err error) int { fmt.Fprintln(errOut, "style count:", err); return ExitInvalid }
	options, path, jsonMode := countOptions(parsed), parsed.Arg(0), parsed.Bool("--json")
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
