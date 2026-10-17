package claude

import (
	"path/filepath"

	"github.com/speakeasy-api/agenthooks/install"
)

func (r *route) ManagementPaths() ([]string, error) {
	paths, err := install.ManagementPaths(r.manifest(nil), r.target())
	return append(paths, filepath.Join(r.dir, "CLAUDE.md"), filepath.Join(r.dir, "rules")), err
}
