package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

func ledgerObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("ledger JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	opening, err := d.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, errors.New("ledger JSON requires an object")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || fields[key] != nil {
			return nil, errors.New("duplicate ledger JSON field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if closing, err := d.Token(); err != nil || closing != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid ledger JSON object")
	}
	return fields, nil
}
