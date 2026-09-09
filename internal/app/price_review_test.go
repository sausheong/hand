package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPriceReviewStrictBoundedInput(t *testing.T) {
	for name, body := range map[string]string{"empty_table": `{"version":1,"prices":[]}`, "missing": `{"version":1}`, "null": `{"version":1,"prices":null}`, "version": `{"version":2,"prices":[]}`, "unknown": `{"version":1,"prices":[],"secret":1}`, "trailing": `{"version":1,"prices":[]} {}`, "invalid_tariff": `{"version":1,"prices":[{}]}`, "oversize": strings.Repeat(" ", 48<<10) + `{}`} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "prices.json")
			if err := os.WriteFile(p, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			review, err := ReadPriceReview(p)
			if (err == nil) != (name == "empty_table") {
				t.Fatalf("review %+v err %v", review, err)
			}
			if err == nil && len(review.Digest) != 64 {
				t.Fatal("missing content digest")
			}
		})
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "table")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(path, []byte(`{"version":1,"prices":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, link} {
		if _, err := ReadPriceReview(p); err == nil {
			t.Fatal("nonregular/symlink file accepted")
		}
	}
}
