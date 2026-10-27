package codex

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/speakeasy-api/agenthooks/install"
)

const runtimePluginID = "style-runtime@style"

// Native registration belongs to STYLE only when its marketplace still points
// at this installation. A missing marketplace can leave a plugin/cache orphan.
func (r *route) removePluginRegistration(data []byte) ([]byte, error) {
	config, err := parseStaticTOML(data)
	if err != nil {
		return nil, fmt.Errorf("codex uninstall: parse config: %w", err)
	}
	marketplaces, _ := config["marketplaces"].(map[string]any)
	if raw, exists := marketplaces[pluginMarketplace]; exists {
		market, _ := raw.(map[string]any)
		source, _ := market["source"].(string)
		if market["source_type"] != "local" || source == "" || canonicalRoutePath(source) != canonicalRoutePath(r.marketplaceRoot()) {
			return nil, fmt.Errorf("codex uninstall: marketplace %q does not belong to this STYLE installation", pluginMarketplace)
		}
	}
	targets := [][]string{{"plugins", runtimePluginID}}
	plugins, _ := config["plugins"].(map[string]any)
	shared := false
	for id := range plugins {
		if id != runtimePluginID && strings.HasSuffix(id, "@"+pluginMarketplace) {
			shared = true
		}
	}
	if !shared {
		targets = append(targets, []string{"marketplaces", pluginMarketplace})
	}
	return removeRegistrationTOML(data, targets)
}

func canonicalRoutePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	if parent := filepath.Dir(abs); parent != abs {
		return filepath.Join(canonicalRoutePath(parent), filepath.Base(abs))
	}
	return abs
}

// Remove table headers and assignments by parsed keys, retaining the original
// bytes of unrelated expressions (including multiline strings and comments).
func removeRegistrationTOML(data []byte, targets [][]string) ([]byte, error) {
	type expression struct {
		start  int
		remove bool
	}
	var expressions []expression
	var table []string
	parser := unstable.Parser{KeepComments: true}
	parser.Reset(data)
	for parser.NextExpression() {
		node := parser.Expression()
		start, remove := int(node.Raw.Offset), false
		if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable || node.Kind == unstable.KeyValue {
			var key []string
			iterator := node.Key()
			for iterator.Next() {
				part := iterator.Node()
				if len(key) == 0 {
					start = int(part.Raw.Offset)
				}
				key = append(key, string(part.Data))
			}
			if node.Kind == unstable.KeyValue {
				key = append(append([]string(nil), table...), key...)
			} else {
				table = key
			}
			for _, target := range targets {
				remove = remove || keyPrefix(key, target)
			}
		}
		start = bytes.LastIndexByte(data[:start], '\n') + 1
		expressions = append(expressions, expression{start, remove})
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("codex uninstall: parse registration: %w", err)
	}
	var result []byte
	last := 0
	for i, expression := range expressions {
		if !expression.remove {
			continue
		}
		end := len(data)
		if i+1 < len(expressions) {
			end = expressions[i+1].start
		}
		result = append(result, data[last:expression.start]...)
		last = end
	}
	result = append(result, data[last:]...)
	config, err := parseStaticTOML(result)
	if err != nil {
		return nil, fmt.Errorf("codex uninstall: invalid remaining config: %w", err)
	}
	for _, target := range targets {
		current := config
		for i, key := range target {
			value, exists := current[key]
			if i == len(target)-1 && exists {
				return nil, fmt.Errorf("codex uninstall: cannot remove inline registration %s", strings.Join(target, "."))
			}
			current, _ = value.(map[string]any)
		}
	}
	return result, nil
}

func keyPrefix(key, prefix []string) bool {
	if len(key) < len(prefix) {
		return false
	}
	for i := range prefix {
		if key[i] != prefix[i] {
			return false
		}
	}
	return true
}

func (r *route) UninstallStorage(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.scope != install.ScopePlugin {
		return nil, nil
	}
	cache := filepath.Join(r.configRoot(), "plugins", "cache", pluginMarketplace, "style-runtime")
	// CheckPath validates files; a child path also checks every cache directory.
	if err := install.CheckPath(r.configRoot(), filepath.Join(cache, ".style-uninstall")); err != nil {
		return nil, err
	}
	return []string{cache}, nil
}
