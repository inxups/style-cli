package codex

import "github.com/speakeasy-api/agenthooks/install"

func (r *route) ManagementPaths() ([]string, error) {
	paths, err := install.ManagementPaths(r.manifest(), r.target())
	return append(paths, r.configPath()), err
}
