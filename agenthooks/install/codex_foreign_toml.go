package install

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Codex's native TOML editor can append plugin/project tables before the
// trailing end-marker comment. Only hook-trust tables belong to this region;
// preserve native tables even when their bytes land inside the markers.
func mergeCodexManagedTOML(existing, rendered []byte, owner string) ([]byte, error) {
	foreign, _, _, err := codexForeignTOML(existing, owner)
	if err != nil {
		return nil, err
	}
	if len(foreign) > 0 {
		_, end := tomlMarkers(owner)
		at := bytes.LastIndex(rendered, []byte(end))
		withForeign := append([]byte(nil), rendered[:at]...)
		withForeign = append(withForeign, foreign...)
		rendered = append(withForeign, rendered[at:]...)
	}
	return mergeManagedTOML(existing, rendered), nil
}

func removeCodexManagedTOML(existing []byte, owner string) ([]byte, bool, error) {
	foreign, begin, end, err := codexForeignTOML(existing, owner)
	if err != nil {
		return nil, false, err
	}
	if len(foreign) == 0 {
		removed, changed := removeManagedTOML(existing, owner)
		return removed, changed, nil
	}
	prefix := bytes.TrimSuffix(existing[:begin], []byte("\n"+tomlBoundaryMarker+"\n"))
	removed := append([]byte(nil), prefix...)
	if len(removed) > 0 && removed[len(removed)-1] != '\n' {
		removed = append(removed, '\n')
	}
	removed = append(removed, foreign...)
	return append(removed, existing[end:]...), true, nil
}

func codexForeignTOML(existing []byte, owner string) ([]byte, int, int, error) {
	beginMarker, endMarker := tomlMarkers(owner)
	begin := bytes.Index(existing, []byte(beginMarker))
	if begin < 0 {
		return nil, 0, 0, nil
	}
	inside := begin + len(beginMarker)
	end := bytes.Index(existing[inside:], []byte(endMarker))
	if end < 0 {
		return nil, 0, 0, fmt.Errorf("Codex hook trust region has no end marker; refusing to discard configuration")
	}
	end += inside
	body := existing[inside:end]
	type section struct {
		start int
		owned bool
	}
	var sections []section
	var parser unstable.Parser
	parser.Reset(body)
	for parser.NextExpression() {
		node := parser.Expression()
		if node.Kind != unstable.Table && node.Kind != unstable.ArrayTable {
			continue
		}
		var keys []string
		keyStart := 0
		iterator := node.Key()
		for iterator.Next() {
			key := iterator.Node()
			if len(keys) == 0 {
				keyStart = int(key.Raw.Offset)
			}
			keys = append(keys, string(key.Data))
		}
		lineStart := bytes.LastIndexByte(body[:keyStart], '\n') + 1
		owned := node.Kind == unstable.Table && len(keys) == 3 && keys[0] == "hooks" && keys[1] == "state"
		sections = append(sections, section{start: lineStart, owned: owned})
	}
	if err := parser.Error(); err != nil {
		return nil, 0, 0, fmt.Errorf("parse Codex hook trust region: %w", err)
	}
	var foreign []byte
	for i, section := range sections {
		if section.owned {
			continue
		}
		stop := len(body)
		if i+1 < len(sections) {
			stop = sections[i+1].start
		}
		foreign = append(foreign, body[section.start:stop]...)
	}
	// The generated trust region always ends on a line boundary. Keep native
	// tables separate from the marker when an external edit omitted its LF.
	if len(foreign) > 0 && !strings.HasSuffix(string(foreign), "\n") {
		foreign = append(foreign, '\n')
	}
	return foreign, begin, end + len(endMarker), nil
}
