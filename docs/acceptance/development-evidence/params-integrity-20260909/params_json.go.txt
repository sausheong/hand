package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Parameter schemas use explicit struct fields. Check decoded key spelling
// before encoding/json can collapse duplicates or match case-insensitive aliases.
// Nested payloads retain their method-specific schemas and validators.
func validateParamFields(raw []byte, out any) error {
	if !utf8.Valid(raw) {
		return errors.New("params must be UTF-8")
	}
	typ := reflect.TypeOf(out)
	if typ == nil || typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct {
		return errors.New("params schema requires a struct pointer")
	}
	typ = typ.Elem()
	allowed := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		allowed[name] = true
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("expected params object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok || !allowed[name] || seen[name] {
			return errors.New("unknown or duplicate params field")
		}
		seen[name] = true
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return err
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return errors.New("invalid params object")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected one params object")
	}
	return nil
}
