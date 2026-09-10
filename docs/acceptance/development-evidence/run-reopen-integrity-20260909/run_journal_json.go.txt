package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"unicode/utf8"
)

// Run annotations own their nested schema. Reject duplicate decoded names,
// case aliases and unknown fields before encoding/json can discard ambiguity.
func runObject(raw json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("run journal JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("run journal record requires an object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || !slices.Contains(allowed, key) || fields[key] != nil {
			return nil, errors.New("unknown or duplicate run journal field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("null run journal field")
		}
		fields[key] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid run journal object")
	}
	return fields, nil
}

func decodeRunRecord(raw json.RawMessage, record *RunRecord) error {
	fields, err := runObject(raw, "version", "budget_id", "id", "run_id", "phase", "at", "model", "outcome")
	if err != nil {
		return err
	}
	for _, key := range []string{"version", "id", "run_id", "phase", "at", "model"} {
		if fields[key] == nil {
			return errors.New("required run journal field missing")
		}
	}
	if _, err := runObject(fields["model"], "Profile", "RequestedModel", "ServingModel", "ServingModelKnown", "ContextLimit", "ContextSource", "Truncated"); err != nil {
		return err
	}
	if rawOutcome := fields["outcome"]; rawOutcome != nil {
		outcome, err := runObject(rawOutcome, "status", "reason", "iterations", "verified")
		if err != nil {
			return err
		}
		if len(outcome) != 4 {
			return errors.New("required run outcome field missing")
		}
	}
	return json.Unmarshal(raw, record)
}
