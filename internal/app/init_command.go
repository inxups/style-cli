package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func runInit(ctx context.Context, service *Service, workDir string, args []string, out, errOut io.Writer) int {
	parsed, err := initCommand.parse(args)
	if err != nil {
		return initCommand.usageError(errOut, err)
	}
	if len(parsed.Positional) > 0 && strings.TrimSpace(parsed.Arg(0)) == "" {
		// An empty name is a mistake, not a request for the derived name.
		return initCommand.usageError(errOut, errors.New("package name cannot be empty"))
	}
	result, err := service.Init(ctx, InitRequest{Name: parsed.Arg(0), OutputDir: workDir, Single: parsed.Bool("--single")})
	var usage *UsageError
	if errors.As(err, &usage) {
		return initCommand.usageError(errOut, err)
	}
	if err != nil {
		fmt.Fprintf(errOut, "style init: %v\n", err)
		var conflictErr *ConflictError
		if errors.As(err, &conflictErr) {
			return ExitConflict
		}
		return ExitInvalid
	}
	displayPath := result.Path
	if relative, relErr := filepath.Rel(workDir, result.Path); relErr == nil {
		displayPath = relative
	}
	if result.Single {
		fmt.Fprintf(out, "created single-file package %s\n", displayPath)
	} else {
		fmt.Fprintf(out, "created package %s\n", displayPath)
	}
	return ExitOK
}
