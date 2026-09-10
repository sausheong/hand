package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/sausheong/harness/budget"
)

func priceObject(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("tariff JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("tariff JSON requires an object")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		permitted := false
		for _, name := range allowed {
			if key == name {
				permitted = true
				break
			}
		}
		if !ok || !permitted || fields[key] != nil {
			return nil, errors.New("unknown or duplicate tariff field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("null tariff field")
		}
		fields[key] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid tariff object")
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing tariff JSON")
	}
	return fields, nil
}

func validatePriceReviewJSON(raw []byte) error {
	fields, err := priceObject(raw, "version", "prices")
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return errors.New("tariff input requires version and prices")
	}
	return validatePriceTableJSON(fields["prices"])
}

func validatePriceTableJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("tariff table requires an array")
	}
	var prices []json.RawMessage
	if err := json.Unmarshal(raw, &prices); err != nil {
		return err
	}
	for _, price := range prices {
		if _, err := priceObject(price, "provider", "model", "destination", "currency", "source", "version", "effective_at", "expires_at", "input_nano_per_million", "output_nano_per_million", "cache_write_nano_per_million", "cache_read_nano_per_million", "fixed_nano", "all_charges_bounded"); err != nil {
			return err
		}
	}
	return nil
}

// DecodePriceTable applies the same canonical JSON and tariff validation to
// protocol arrays as file-based reviews, before any session mutation.
func DecodePriceTable(raw []byte) ([]budget.PriceSnapshot, error) {
	if len(raw) > 48<<10 {
		return nil, errors.New("tariff table exceeds 48 KiB")
	}
	if err := validatePriceTableJSON(raw); err != nil {
		return nil, err
	}
	var prices []budget.PriceSnapshot
	if err := json.Unmarshal(raw, &prices); err != nil {
		return nil, err
	}
	review, err := reviewPrices(prices)
	if err != nil {
		return nil, err
	}
	return review.Prices, nil
}
