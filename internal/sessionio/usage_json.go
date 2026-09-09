package sessionio

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"unicode/utf8"
)

// Usage annotations own their nested schema. Reject duplicate decoded names,
// case aliases and unknown fields before encoding/json can discard ambiguity.
func usageObject(raw json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("usage JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("usage record requires an object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || !slices.Contains(allowed, key) || fields[key] != nil {
			return nil, errors.New("unknown or duplicate usage field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid usage object")
	}
	return fields, nil
}

func decodeUsageRecord(raw json.RawMessage, record *usageRecord) error {
	fields, err := usageObject(raw, "version", "request")
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return errors.New("usage record fields missing")
	}
	request, err := usageObject(fields["request"], "request_id", "model", "category", "status", "source", "usage")
	if err != nil {
		return err
	}
	if len(request) != 6 {
		return errors.New("usage request fields missing")
	}
	if !bytes.Equal(bytes.TrimSpace(request["usage"]), []byte("null")) {
		usage, err := usageObject(request["usage"], "input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")
		if err != nil {
			return err
		}
		if usage["input_tokens"] == nil || usage["output_tokens"] == nil {
			return errors.New("reported usage totals missing")
		}
		for _, value := range usage {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return errors.New("reported usage token count is null")
			}
		}
	}
	return json.Unmarshal(raw, record)
}

func decodeUsageTracking(raw json.RawMessage, record *usageTracking) error {
	fields, err := usageObject(raw, "version", "prior_usage_unknown")
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return errors.New("usage tracking fields missing")
	}
	return json.Unmarshal(raw, record)
}
