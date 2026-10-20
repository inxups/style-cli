package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/inxups/style-cli/internal/saferead"
	"github.com/inxups/style-cli/internal/tokencount"
)

// countOptions maps --exact provider:target onto explicit counter options;
// without it counts stay local estimates. Provider is never inferred.
func countOptions(parsed parsedArgs) (tokencount.Options, error) {
	if !parsed.Has("--exact") {
		return tokencount.Options{}, nil
	}
	provider, target, ok := strings.Cut(parsed.String("--exact"), ":")
	options := tokencount.Options{Mode: "exact", Provider: provider}
	switch {
	case !ok || target == "":
		return options, fmt.Errorf("--exact requires openai:<encoding> or anthropic:<model>")
	case provider == "openai":
		options.Encoding = target
	case provider == "anthropic":
		options.Model = target
	default:
		return options, fmt.Errorf("--exact provider must be openai or anthropic")
	}
	return options, nil
}
func runCount(ctx context.Context, s *Service, workDir string, args []string, out, errOut io.Writer) int {
	parsed, err := countCommand.parse(args)
	if err != nil {
		return countCommand.usageError(errOut, err)
	}
	options, err := countOptions(parsed)
	if err != nil {
		return countCommand.usageError(errOut, err)
	}
	fail := func(err error) int {
		fmt.Fprintln(errOut, "style count:", strings.TrimPrefix(err.Error(), "count: "))
		return ExitInvalid
	}
	path, jsonMode := parsed.Arg(0), parsed.Bool("--json")
	if path == "" {
		return countCommand.usageError(errOut, fmt.Errorf("UTF-8 file is required"))
	}
	if err := options.Validate(); err != nil {
		return countCommand.usageError(errOut, err)
	}
	display := path
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	data, err := saferead.ReadFile(path, tokencount.MaxTextBytes)
	if err != nil {
		return fail(fmt.Errorf("%s: %s", display, countReadFailure(path, err)))
	}
	if !utf8.Valid(data) {
		return fail(fmt.Errorf("%s: file is not valid UTF-8", display))
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
	fmt.Fprintf(out, "%d tokens; %s\n%s\n", r.Tokens,
		textFields("mode", r.Mode, "provider", r.Provider, "scope", r.Scope, "encoding", r.Encoding, "model", r.Model),
		textFields("text_sha256", r.TextSHA256, "implementation", r.Implementation, "api_version", r.APIVersion))
}

// textFields joins key=value pairs for text reports, leaving out empty values.
func textFields(pairs ...string) string {
	fields := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			fields = append(fields, pairs[i]+"="+pairs[i+1])
		}
	}
	return strings.Join(fields, " ")
}

// countReadFailure says why a file could not be counted without echoing the
// underlying system error.
func countReadFailure(path string, err error) string {
	info, statErr := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(statErr, fs.ErrNotExist):
		return "file does not exist"
	case statErr == nil && !info.Mode().IsRegular():
		return "not a regular file"
	case statErr == nil && info.Size() > tokencount.MaxTextBytes:
		return fmt.Sprintf("file exceeds %d bytes", tokencount.MaxTextBytes)
	default:
		return "cannot read file"
	}
}
