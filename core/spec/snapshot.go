package spec

import (
	"bytes"
	"fmt"
	"sort"
	"unicode/utf8"
)

// LoadSnapshot parses a private copy of a complete package snapshot. It never
// reads the filesystem, including during validation. singleFile is empty for a
// directory package, or the sole source filename for a single-file package.
// root is only a diagnostic label.
func LoadSnapshot(root string, input map[string][]byte, singleFile string) (*Package, error) {
	files := make(map[string][]byte, len(input))
	names := make([]string, 0, len(input))
	for name, data := range input {
		clean, err := cleanRelativePath(name)
		if err != nil {
			return nil, err
		}
		if clean != name {
			return nil, parseErr("path-escape", name, "snapshot paths must be canonical", nil)
		}
		if len(data) > MaxFileSize {
			return nil, parseErr("file-too-large", name, fmt.Sprintf("file exceeds %d bytes", MaxFileSize), nil)
		}
		files[name] = append([]byte(nil), data...)
		names = append(names, name)
	}
	sort.Strings(names)
	read := func(name string) ([]byte, error) {
		clean, err := cleanRelativePath(name)
		if err != nil {
			return nil, err
		}
		data, ok := files[clean]
		if !ok {
			return nil, parseErr("file-not-found", clean, "file is absent from package snapshot", nil)
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return nil, parseErr("bad-encoding", clean, "file must be UTF-8 text without NUL bytes", nil)
		}
		return data, nil
	}
	var pkg *Package
	var err error
	if singleFile != "" {
		if len(files) != 1 {
			return nil, parseErr("invalid-package", singleFile, "single-file snapshot must contain exactly one file", nil)
		}
		data, readErr := read(singleFile)
		if readErr != nil {
			return nil, readErr
		}
		pkg, err = loadSingleBytes(root, singleFile, files, data)
	} else {
		pkg, err = loadDirectoryFrom(root, files, packageReader{read: read, expand: func(pattern string) ([]string, error) { return expandFromNames(pattern, names) }})
	}
	if err != nil {
		return nil, err
	}
	pkg.inventory = names
	return pkg, nil
}
