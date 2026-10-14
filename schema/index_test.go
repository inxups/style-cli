package schema

import (
	"os"
	"testing"
)

func TestValidateIndexUsesCommittedSchema(t *testing.T) {
	data, err := os.ReadFile("../testdata/golden/index.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateIndex(data); err != nil {
		t.Fatalf("valid golden rejected: %v", err)
	}
	for _, invalid := range [][]byte{
		[]byte("{\"format\":2}"),
		[]byte("{\"format\":1,\"unexpected\":true}"),
		[]byte("{"),
	} {
		if err := ValidateIndex(invalid); err == nil {
			t.Errorf("invalid document accepted: %s", invalid)
		}
	}
}
