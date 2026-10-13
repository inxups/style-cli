package spec

import (
	"bytes"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func parseYAMLMap(data []byte, source string) (map[string]any, error) {
	var fields map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&fields); err != nil {
		return nil, parseErr("invalid-yaml", source, "parse YAML", err)
	}
	if fields == nil {
		return nil, parseErr("invalid-yaml", source, "expected a YAML mapping", nil)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, parseErr("invalid-yaml", source, "expected exactly one YAML document", err)
	}
	return fields, nil
}

func splitFrontmatter(data []byte, source string) ([]byte, string, error) {
	text := string(data)
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, "", parseErr("bad-encoding", source, "file must be UTF-8 text without NUL bytes", nil)
	}
	start := 0
	if strings.HasPrefix(text, "\ufeff") {
		start = len("\ufeff")
	}
	first, next := nextLine(text, start)
	if trimLineEnding(first) != "---" {
		return nil, "", parseErr("invalid-frontmatter", source, "expected opening --- delimiter", nil)
	}
	yamlStart := next
	lineStart := next
	for lineStart <= len(text) {
		line, after := nextLine(text, lineStart)
		if trimLineEnding(line) == "---" {
			yamlBytes := []byte(text[yamlStart:lineStart])
			body := text[after:]
			body = trimLeadingBlankLines(body)
			body = strings.TrimRightFunc(body, unicode.IsSpace)
			return yamlBytes, body, nil
		}
		if after == lineStart || after >= len(text) {
			break
		}
		lineStart = after
	}
	return nil, "", parseErr("invalid-frontmatter", source, "missing closing --- delimiter", nil)
}

func hasFrontmatter(data []byte) bool {
	text := string(data)
	start := 0
	if strings.HasPrefix(text, "\ufeff") {
		start = len("\ufeff")
	}
	first, _ := nextLine(text, start)
	return trimLineEnding(first) == "---"
}

func nextLine(text string, start int) (string, int) {
	if start >= len(text) {
		return "", len(text)
	}
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		return text[start:], len(text)
	}
	end += start
	return text[start:end], end + 1
}

func trimLineEnding(line string) string { return strings.TrimSuffix(line, "\r") }

func trimLeadingBlankLines(body string) string {
	for body != "" {
		line, after := nextLine(body, 0)
		if strings.TrimSpace(trimLineEnding(line)) != "" {
			return body
		}
		if after == len(body) {
			return ""
		}
		body = body[after:]
	}
	return body
}
