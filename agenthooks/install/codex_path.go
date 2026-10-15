package install

import (
	"errors"
	"os"
	"path/filepath"
)

// Resolve existing ancestors too: a new CODEX_HOME under /var or another
// symlinked parent must get the same trust key before and after creation.
func canonicalCodexDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	current := abs
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			suffix, _ := filepath.Rel(current, abs)
			return filepath.Join(resolved, suffix)
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(current) == current {
			return abs
		}
		current = filepath.Dir(current)
	}
}
