package claude

import (
	"bytes"
	"testing"

	"github.com/inxups/style-cli/internal/adapter"
)

func TestStaticSeparatorRestoresBytesAfterEitherOwnerRemovalOrder(t *testing.T) {
	for _, original := range []string{"User text.", "User text.\n", "User text.\n\n", "User text.\r\n", "  ", "\n"} {
		for _, order := range [][]adapter.Owner{{"a", "b"}, {"b", "a"}} {
			t.Run(original+"/"+string(order[0]), func(t *testing.T) {
				blocks := []adapter.StaticBlock{{Owner: "a", Body: "A"}, {Owner: "b", Body: "B"}}
				installed, err := mergeClaudeMD([]byte(original), blocks)
				if err != nil {
					t.Fatal(err)
				}
				again, err := mergeClaudeMD(installed, blocks)
				if err != nil || !bytes.Equal(installed, again) {
					t.Fatalf("non-idempotent static merge: %q %q %v", installed, again, err)
				}
				for _, owner := range order {
					installed, err = removeClaudeMD(installed, string(owner))
					if err != nil {
						t.Fatal(err)
					}
					if _, err := parseClaudeRegions(string(installed)); err != nil {
						t.Fatal(err)
					}
				}
				if string(installed) != original {
					t.Fatalf("restored %q want %q", installed, original)
				}
			})
		}
	}
}
