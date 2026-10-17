package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inxups/style-cli/core/compile"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/internal/adapter"
	"gopkg.in/yaml.v3"
)

const effortMappingLimit = 64 << 10

// captureEffortMapping adds the user file to the same immutable input capture as
// package/lock/config candidates, without putting user policy in package data.
func (s *Service) captureEffortMapping(home string, inputs *managementInputs) (compile.EffortMapping, error) {
	path, err := filepath.Abs(filepath.Join(home, "effort.yaml"))
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("effort-mapping-invalid: cannot inspect configuration")
	}
	if err == nil && (!info.Mode().IsRegular() || info.Size() > effortMappingLimit) {
		return nil, fmt.Errorf("effort-mapping-invalid: expected a regular file of at most 64 KiB")
	}
	if inputs == nil {
		inputs = &managementInputs{trees: map[string]inputTree{}, parents: map[string]os.FileInfo{}}
	}
	if err = inputs.add(path); err != nil {
		return nil, err
	}
	file, exists := inputs.trees[path].files["."]
	if !exists {
		return nil, nil
	}
	if !file.info.Mode().IsRegular() || len(file.data) > effortMappingLimit {
		return nil, fmt.Errorf("effort-mapping-invalid: expected a bounded regular file")
	}
	var config struct {
		Format    int                          `yaml:"format"`
		Harnesses map[string]map[string]string `yaml:"harnesses"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(file.data))
	decoder.KnownFields(true)
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF || config.Format != 1 {
		return nil, fmt.Errorf("effort-mapping-invalid: expected one strict format 1 document")
	}
	mapping := compile.EffortMapping{}
	for h, rows := range config.Harnesses {
		if _, ok := s.definitions[adapter.ProviderID(h)]; !ok {
			return nil, fmt.Errorf("effort-mapping-invalid: unregistered harness")
		}
		if len(rows) > 128 {
			return nil, fmt.Errorf("effort-mapping-invalid: too many aliases")
		}
		if len(rows) == 0 {
			continue
		}
		mapping[h] = map[string]string{}
		for raw, tier := range rows {
			key := strings.ToLower(strings.TrimSpace(raw))
			if _, exists := mapping[h][key]; exists {
				return nil, fmt.Errorf("effort-mapping-invalid: normalized aliases conflict")
			}
			mapping[h][key] = strings.ToLower(strings.TrimSpace(tier))
		}
	}
	if err = mapping.Validate(); err != nil {
		return nil, fmt.Errorf("effort-mapping-invalid: invalid alias or canonical tier")
	}
	return mapping.Clone(), nil
}
func effortMappingDiagnostics(mapping compile.EffortMapping) []Diagnostic {
	var out []Diagnostic
	names := make([]string, 0, len(mapping))
	for h := range mapping {
		names = append(names, h)
	}
	sort.Strings(names)
	for _, h := range names {
		out = append(out, Diagnostic{Code: "effort-mapping", Provider: h, Severity: spec.SeverityWarning, Message: "compiled user effort mapping " + mapping.SHA256(h) + "; explicit aliases override builtin mappings and are approximate"})
	}
	return out
}
