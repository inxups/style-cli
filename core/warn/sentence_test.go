package warn

import (
	"testing"

	"github.com/inxups/style-cli/core/template"
)

func TestFirstSentence(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "leading blank lines and Chinese full stop",
			text: "\n \t\n  请先检查改动。后面的内容不发送给模型。\n第三行。",
			want: "请先检查改动。",
		},
		{
			name: "Chinese exclamation mark",
			text: "注意！后续。",
			want: "注意！",
		},
		{
			name: "Chinese question mark",
			text: "继续吗？请确认。",
			want: "继续吗？",
		},
		{
			name: "English punctuation followed by whitespace",
			text: "Check the patch! Then run tests? Yes. More text.",
			want: "Check the patch!",
		},
		{
			name: "ASCII punctuation at end of line",
			text: "Run tests first.\nThen review the result.",
			want: "Run tests first.",
		},
		{
			name: "ASCII question mark followed by whitespace",
			text: "Ready?\tContinue when prepared.",
			want: "Ready?",
		},
		{
			name: "period inside a number is not a boundary",
			text: "Version 2.1 is current. More details follow.",
			want: "Version 2.1 is current.",
		},
		{
			name: "first non-empty line without punctuation",
			text: "\n  Review the changed files\nRun tests and inspect the diff.",
			want: "Review the changed files",
		},
		{
			name: "multiline content stops at first line",
			text: "Keep this change small\nand verify it with tests.",
			want: "Keep this change small",
		},
		{
			name: "CRLF and trailing whitespace",
			text: "\r\n  Run the checks.  \r\nMore details.",
			want: "Run the checks.",
		},
		{
			name: "no terminal punctuation",
			text: "No punctuation here",
			want: "No punctuation here",
		},
		{
			name: "only blank lines",
			text: "\n \t\r\n",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FirstSentence(test.text); got != test.want {
				t.Fatalf("FirstSentence(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestFirstSentenceAfterDynamicTemplateRender(t *testing.T) {
	parsed, err := template.Parse("Review {{tool.input.path}} before merging. Details follow.")
	if err != nil {
		t.Fatal(err)
	}
	rendered := parsed.Render(map[string]string{"tool.input.path": "core/warn/sentence.go"})
	if got, want := FirstSentence(rendered), "Review core/warn/sentence.go before merging."; got != want {
		t.Fatalf("FirstSentence(rendered) = %q, want %q", got, want)
	}
}
