package claude

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/speakeasy-api/agenthooks/install"
)

func (r *route) ManagementPaths() ([]string, error) {
	paths, err := install.ManagementPaths(r.manifest(nil), r.target())
	if r.plugin {
		paths = append(paths, filepath.Join(r.dir, "settings.json"), filepath.Join(r.dir, "plugins/known_marketplaces.json"), filepath.Join(r.dir, "plugins/installed_plugins.json"))
	}
	return append(paths, filepath.Join(r.dir, "CLAUDE.md"), filepath.Join(r.dir, "rules")), err
}

// ProjectDigest omits a missing settings.json: a project with only static
// rules never plans hooks, so the file need not exist.
func (r *route) ProjectDigest(read func(string) ([]byte, error)) (map[string]string, error) {
	digest, err := install.ProjectDigest(r.manifest(nil), r.target(), read)
	if errors.Is(err, os.ErrNotExist) && r.project {
		if _, e := read(filepath.Join(r.dir, "settings.json")); errors.Is(e, os.ErrNotExist) {
			return map[string]string{}, nil
		}
	}
	return digest, err
}
