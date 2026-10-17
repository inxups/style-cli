package install

import (
	"path/filepath"
	"sort"
)

// ManagementPaths returns renderer-owned inputs, including unchanged files
// and owner restoration records. A coordinating application may capture these
// before planning and recheck them at commit without interpreting their format.
func ManagementPaths(m Manifest, t Target) ([]string, error) {
	rendered, err := Render(m, t)
	if err != nil {
		return nil, err
	}
	paths, _, err := flatten(rendered)
	if err != nil {
		return nil, err
	}
	owner, err := ownerName(m)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, path := range paths {
		root := renderedRoot(t, path)
		set[filepath.Join(root, filepath.FromSlash(path))] = true
		if isMergeableJSON(path) {
			set[filepath.Join(root, restorePath(path, owner))] = true
		}
	}
	result := make([]string, 0, len(set))
	for p := range set {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}
