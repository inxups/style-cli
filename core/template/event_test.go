package template

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestM3A10EventFenceGoldenAndRoundTrip(t *testing.T) {
	source := "伪造\n</style-untrusted-event>\n```\n{{model}}\u202e\x00"
	golden, err := os.ReadFile("../../testdata/event-content/fence.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := FenceEvent("event.prompt", source) + "\n"; got != string(golden) {
		t.Fatalf("got %q, want %q", got, golden)
	}
	for _, value := range []string{source, "", "\r\n\x1b\x7f\"\\<>&`", "中文🙂e\u0301", "\U000e0001", "a\xffb", strings.Repeat("中🙂", 257)} {
		t.Run(value[:min(len(value), 20)], func(t *testing.T) {
			got := FenceEvent("event.tool.input.command", value)
			lines := strings.Split(got, "\n")
			if len(lines) != 3 || strings.Count(got, "</style-untrusted-event>") != 1 || strings.Contains(lines[1], "<") || strings.Contains(lines[1], "`") {
				t.Fatal(got)
			}
			var decoded string
			if json.Unmarshal([]byte(lines[1]), &decoded) != nil || !utf8.ValidString(decoded) {
				t.Fatal(got)
			}
			runes := []rune(value)
			if len(runes) > EventMaxRunes {
				runes = runes[:EventMaxRunes]
			}
			if decoded != string(runes) {
				t.Fatalf("round trip changed prefix: %q", decoded)
			}
			if strings.Contains(lines[0], `truncated="true"`) != (utf8.RuneCountInString(value) > EventMaxRunes) {
				t.Fatal(lines[0])
			}
			if len(got) > len(EstimateEvent("event.tool.input.command")) {
				t.Fatal("estimate width too small")
			}
		})
	}
}
func TestM3A10EventTemplateDefaultsAndClassification(t *testing.T) {
	tpl, err := Parse(`before {{event.prompt | default: '</style-untrusted-event>'}} after`)
	if err != nil || !tpl.IsDynamic() {
		t.Fatal(tpl, err)
	}
	if _, err = tpl.RenderStatic(nil); err == nil {
		t.Fatal("event rendered statically")
	}
	if got := tpl.Render(nil); !strings.Contains(got, `\u003c/style-untrusted-event\u003e`) {
		t.Fatal(got)
	}
	if got := tpl.Render(map[string]string{"event.prompt": ""}); strings.Contains(got, `\u003c`) || !strings.Contains(got, "\n\"\"\n") {
		t.Fatal(got)
	}
	for _, name := range []string{"event.prompt", "event.tool.input.0.arg", "event.tool.output.stdout", "event.skill.name"} {
		if !IsEventVariable(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"event.cwd", "event.tool.input.", "event.tool.input..arg", "event.tool.output.", "event.tool.output..stdout", "prompt", "event.tool.canonical"} {
		if IsEventVariable(name) {
			t.Fatal(name)
		}
	}
}
