package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sausheong/harness/budget"
)

func FuzzPriceReviewJSON(f *testing.F) {
	for _, seed := range []string{
		`{"version":1,"prices":[]}`,
		`{"version":1,"prices":[{"provider":"local","model":"test","destination":"fixture","currency":"USD","source":"test","version":"1","effective_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","fixed_nano":1,"all_charges_bounded":true}]}`,
		`{"version":1,"prices":null}`,
		`{"version":2,"version":1,"prices":[]}`,
		`{"Version":1,"prices":[]}`,
		`{"version":1,"prices":[{"fixed_nano":100,"fixed_\u006eano":0}]}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 48<<10 {
			return
		}
		if err := validatePriceReviewJSON(raw); err != nil {
			return
		}
		if !json.Valid(raw) {
			t.Fatal("accepted invalid JSON")
		}
		// Validated documents have a top-level version. A second spelling of
		// that decoded key must never be collapsed by a permissive decoder.
		trimmed := bytes.TrimSpace(raw)
		for _, key := range []string{`"version"`, `"\u0076ersion"`} {
			duplicate := append([]byte("{"+key+":1,"), trimmed[1:]...)
			if err := validatePriceReviewJSON(duplicate); err == nil {
				t.Fatal("duplicate key accepted")
			}
		}
		var doc struct {
			Version int                    `json:"version"`
			Prices  []budget.PriceSnapshot `json:"prices"`
		}
		if json.Unmarshal(raw, &doc) != nil || doc.Version != 1 {
			return
		}
		review, err := reviewPrices(doc.Prices)
		if err != nil {
			return
		}
		doc.Prices = review.Prices
		canonical, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := validatePriceReviewJSON(canonical); err != nil {
			t.Fatalf("canonical review rejected: %v", err)
		}
		var again struct {
			Version int                    `json:"version"`
			Prices  []budget.PriceSnapshot `json:"prices"`
		}
		if err := json.Unmarshal(canonical, &again); err != nil {
			t.Fatal(err)
		}
		replay, err := reviewPrices(again.Prices)
		if err != nil || replay.Digest != review.Digest {
			t.Fatalf("review digest changed across canonical round trip: %v", err)
		}
	})
}
