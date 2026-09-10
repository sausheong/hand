package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/sausheong/harness/budget"
)

// PriceReview contains the exact bounded table presented for confirmation.
// The digest identifies reviewed content; it is not a signature or price audit.
type PriceReview struct {
	Prices []budget.PriceSnapshot
	Digest string
}

func reviewPrices(prices []budget.PriceSnapshot) (PriceReview, error) {
	if len(prices) > 16 {
		return PriceReview{}, errors.New("at most 16 route tariffs are allowed")
	}
	type key struct{ provider, model, destination string }
	seen := map[key]bool{}
	for _, p := range prices {
		if err := p.Validate(); err != nil {
			return PriceReview{}, err
		}
		k := key{p.Provider, p.Model, p.Destination}
		if seen[k] {
			return PriceReview{}, errors.New("duplicate tariff route")
		}
		seen[k] = true
	}
	// Normalise an explicitly empty table so its digest is stable.
	copy := append([]budget.PriceSnapshot{}, prices...)
	raw, err := json.Marshal(copy)
	if err != nil {
		return PriceReview{}, err
	}
	if len(raw) > 48<<10 {
		return PriceReview{}, errors.New("tariff table exceeds 48 KiB")
	}
	sum := sha256.Sum256(raw)
	return PriceReview{Prices: copy, Digest: hex.EncodeToString(sum[:])}, nil
}
func ReadPriceReview(path string) (PriceReview, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return PriceReview{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return PriceReview{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 48<<10 {
		return PriceReview{}, errors.New("tariff input must be a regular file at most 48 KiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (48<<10)+1))
	if err != nil {
		return PriceReview{}, err
	}
	if len(raw) > 48<<10 {
		return PriceReview{}, errors.New("tariff input exceeds 48 KiB")
	}
	if err := validatePriceReviewJSON(raw); err != nil {
		return PriceReview{}, err
	}
	var doc struct {
		Version int                     `json:"version"`
		Prices  *[]budget.PriceSnapshot `json:"prices"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&doc); err != nil {
		return PriceReview{}, err
	}
	if d.Decode(new(any)) != io.EOF || doc.Version != 1 || doc.Prices == nil {
		return PriceReview{}, errors.New("tariff input requires version 1 and an explicit prices array")
	}
	return reviewPrices(*doc.Prices)
}
func (c *Controller) ApplyPriceReview(ctx context.Context, review PriceReview) error {
	checked, err := reviewPrices(review.Prices)
	if err != nil {
		return err
	}
	if review.Digest == "" || checked.Digest != review.Digest {
		return errors.New("tariff review digest mismatch")
	}
	return c.SetCostPrices(ctx, checked.Prices)
}
