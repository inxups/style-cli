package state

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatsCostMetadataIsBoundedAndRejectsNonHashes(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "state"))
	tokens := 1234
	record := StatsRecord{At: time.Now().Format(time.RFC3339Nano), Provider: "claude-code",
		Event: "UserPromptExpansion", Point: "skill.explicit", Entry: strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64) + "@skill.explicit",
		Action: "inject", Result: "output", Tokens: &tokens, BodySHA256: "sha256:" + strings.Repeat("0", 64)}
	data, _ := json.Marshal(record)
	if err := store.AppendStats("session", "main", data); err != nil {
		t.Fatalf("maximum-length rule metadata (%d bytes): %v", len(data), err)
	}
	record.BodySHA256 = "arbitrary private text"
	data, _ = json.Marshal(record)
	if err := store.AppendStats("session", "main", data); err == nil {
		t.Fatal("non-hash body metadata accepted")
	}
	record.BodySHA256, tokens = "", -1
	data, _ = json.Marshal(record)
	if err := store.AppendStats("session", "main", data); err == nil {
		t.Fatal("negative token estimate accepted")
	}
}
