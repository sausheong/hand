package sdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Check known schema names without disallowing future event extensions. Duplicate
// decoded names are ambiguous even when the current SDK does not use the field.
func validateSnapshotObject(raw json.RawMessage, canonical ...string) error {
	if !utf8.Valid(raw) {
		return errors.New("snapshot JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return errors.New("snapshot requires an object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return errors.New("duplicate snapshot field")
		}
		seen[key] = true
		for _, name := range canonical {
			if key != name && strings.EqualFold(key, name) {
				return errors.New("noncanonical snapshot field")
			}
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return err
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return errors.New("invalid snapshot object")
	}
	return nil
}
