package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPriceReviewRejectsAmbiguousFields(t *testing.T) {
	price := `{"provider":"local","model":"test","destination":"local","currency":"USD","source":"fixture","version":"1","effective_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","fixed_nano":1,"all_charges_bounded":true}`
	valid := `{"version":1,"prices":[` + price + `]}`
	cases := map[string]string{
		"duplicate_version": strings.Replace(valid, `"version":1`, `"version":2,"version":1`, 1),
		"version_alias":     strings.Replace(valid, `"version":1`, `"Version":1`, 1),
		"duplicate_prices":  strings.Replace(valid, `"prices":`, `"prices":[],"prices":`, 1),
		"escaped_duplicate": strings.Replace(valid, `"fixed_nano":1`, `"fixed_nano":100,"fixed_\u006eano":1`, 1),
		"rate_alias":        strings.Replace(valid, `"fixed_nano":1`, `"FIXED_NANO":1`, 1),
		"null_rate":         strings.Replace(valid, `"fixed_nano":1`, `"fixed_nano":null`, 1),
		"duplicate_bounded": strings.Replace(valid, `"all_charges_bounded":true`, `"all_charges_bounded":false,"all_charges_bounded":true`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "prices.json")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			review, err := ReadPriceReview(path)
			if err == nil || review.Digest != "" {
				t.Fatal("ambiguous tariff received approval digest")
			}
			raw, err := os.ReadFile(path)
			if err != nil || string(raw) != body {
				t.Fatal("review changed input")
			}
			if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
				t.Fatal(err)
			}
			review, err = ReadPriceReview(path)
			if err != nil || len(review.Prices) != 1 || review.Prices[0].FixedNano != 1 {
				t.Fatalf("valid repaired input rejected: %v", err)
			}
		})
	}
}
