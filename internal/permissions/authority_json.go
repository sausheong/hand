package permissions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// Authority records are private security decisions. Reject ambiguous JSON before
// Go's permissive struct decoder can collapse duplicate keys or case aliases.
func validateAuthorityJSON(raw []byte) error {
	record, err := authorityJSONObject(raw, []string{"version", "grant", "revoke"}, []string{"version"})
	if err != nil {
		return err
	}
	if grant, ok := record["grant"]; ok {
		fields := []string{"ID", "Operation", "Scope", "Resource", "Lifetime", "SessionID", "InvocationID", "Provenance", "Workspace", "ConfigDigest"}
		if _, err = authorityJSONObject(grant, fields, fields); err != nil {
			return err
		}
	}
	return nil
}

func authorityJSONObject(raw []byte, allowed, required []string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("authority JSON must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, errors.New("authority JSON must be an object")
	}
	names := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		names[name] = true
	}
	values := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok || !names[name] {
			return nil, errors.New("unknown or noncanonical authority field")
		}
		if _, ok = values[name]; ok {
			return nil, errors.New("duplicate authority field")
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return nil, err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("null authority field")
		}
		values[name] = value
	}
	if _, err = decoder.Token(); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing authority JSON")
	}
	for _, name := range required {
		if _, ok := values[name]; !ok {
			return nil, errors.New("missing authority field")
		}
	}
	return values, nil
}
