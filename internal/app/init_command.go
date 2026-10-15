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
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "style init [name] [--single]")
		return ExitOK
	}
	name, single, err := parseInitArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "style init: %v\n", err)
		return ExitUsage
	}
	result, err := service.Init(ctx, InitRequest{Name: name, OutputDir: workDir, Single: single})
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

func parseInitArgs(args []string) (string, bool, error) {
	var name string
	single := false
	for _, arg := range args {
		switch arg {
		case "--single":
			single = true
		case "--help", "-h":
			return "", false, errors.New("usage: style init [name] [--single]")
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, fmt.Errorf("unknown option %q", arg)
			}
			if name != "" {
				return "", false, errors.New("only one package name is allowed")
			}
			name = arg
		}
	}
	return name, single, nil
}
