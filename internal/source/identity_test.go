package source

import (
	"strings"
	"testing"
)

func TestM3A3AdvertisementIntegrity(t *testing.T) {
	oid, other := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, c := range []struct{ name, advert, requested string }{
		{"unborn-head", "", ""}, {"detached-head", oid + "\tHEAD\n", ""},
		{"ambiguous-head", "ref: refs/heads/main\tHEAD\nref: refs/heads/other\tHEAD\n" + oid + "\tHEAD\n", ""},
		{"duplicate-ref", oid + "\trefs/heads/main\n" + other + "\trefs/heads/main\n", "refs/heads/main"},
		{"noncanonical-head", "ref: main\tHEAD\n" + oid + "\tHEAD\n", ""},
		{"missing-ref", oid + "\trefs/heads/other\n", "refs/heads/main"},
		{"object-format-mismatch", strings.Repeat("c", 64) + "\tHEAD\n", oid},
		{"peel-format-mismatch", oid + "\trefs/tags/v1\n" + strings.Repeat("c", 64) + "\trefs/tags/v1^{}\n", "refs/tags/v1"},
		{"malformed-advert", "remote CANARY\n", "refs/heads/main"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, _, _, err := parseAdvertisement([]byte(c.advert), c.requested)
			if Code(err) != "source-invalid" || strings.Contains(err.Error(), "CANARY") {
				t.Fatal(err)
			}
		})
	}
}

func TestM3A3GitIdentityCombinations(t *testing.T) {
	files := map[string][]byte{"review.style.md": []byte("text\n")}
	hashes, content := Hashes(files)
	g := Git{URL: "https://example.invalid/repo.git", Ref: "refs/heads/main", ObjectFormat: "sha1", Commit: strings.Repeat("a", 40), Object: strings.Repeat("b", 40), ContentSHA256: content}
	if err := ValidateIdentity(g, "file", "nested/review.style.md", hashes); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, kind, p string
		files         map[string]string
	}{
		{"wrong-file-basename", "file", "other.style.md", hashes}, {"multi-file", "file", "review.style.md", map[string]string{"review.style.md": hashes["review.style.md"], "extra": "sha256:" + strings.Repeat("a", 64)}}, {"unknown-kind", "git", ".", hashes}, {"empty", "dir", ".", map[string]string{}}, {"invalid-path", "dir", "../pkg", hashes},
	} {
		t.Run(c.name, func(t *testing.T) {
			copy := g
			copy.ContentSHA256 = Aggregate(c.files)
			if err := ValidateIdentity(copy, c.kind, c.p, c.files); err == nil {
				t.Fatal("inconsistent identity accepted")
			}
		})
	}
	for _, c := range []struct {
		name  string
		files map[string][]byte
	}{
		{"unicode-casefold", map[string][]byte{"k/x": []byte("a"), "K/y": []byte("b")}},
		{"directory-casefold", map[string][]byte{"Dir/x": []byte("a"), "dir/y": []byte("b")}},
		{"metadata", map[string][]byte{".git/config": []byte("a")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := validateFiles(c.files); Code(err) != "source-invalid" {
				t.Fatal(err)
			}
		})
	}
}
