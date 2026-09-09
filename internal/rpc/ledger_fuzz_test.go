package rpc

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"unicode/utf8"
)

func FuzzLedgerObjectIntegrity(f *testing.F) {
	for _, seed := range []string{
		`{}`,
		`{"id":"request","state":"pending","fingerprint":"0123456789abcdef"}`,
		`{"result":{"kind":"terminal","payload":{"status":"completed"}}}`,
		`{"id":"first","id":"second"}`,
		`{"id":"first","i\u0064":"second"}`,
		`{"result":null}`,
		`{"id":"request"} {}`,
		"{\"id\":\"\xff\"}",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			return
		}
		fields, err := ledgerObject(raw)
		if err != nil {
			return
		}
		if !utf8.Valid(raw) || !json.Valid(raw) {
			t.Fatal("accepted invalid JSON or UTF-8")
		}
		// Standard decoding gives an independent value oracle after the stricter
		// parser has ruled out duplicate top-level fields.
		var ordinary map[string]json.RawMessage
		if err := json.Unmarshal(raw, &ordinary); err != nil || !reflect.DeepEqual(fields, ordinary) {
			t.Fatal("strict parser changed field values")
		}
		canonical, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		roundtrip, err := ledgerObject(canonical)
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range fields {
			var want, got any
			if json.Unmarshal(value, &want) != nil || json.Unmarshal(roundtrip[key], &got) != nil || !reflect.DeepEqual(want, got) {
				t.Fatal("roundtrip changed journal value")
			}
			encodedKey, _ := json.Marshal(key)
			// Inject a second decoded key with a different value. Accepting either
			// ordering would let an ambiguous journal change request identity/state.
			prefix := append([]byte{'{'}, encodedKey...)
			prefix = append(prefix, []byte(":null,")...)
			duplicate := append(prefix, bytes.TrimSpace(raw)[1:]...)
			if _, err := ledgerObject(duplicate); err == nil {
				t.Fatal("duplicate journal field accepted")
			}
		}
		if _, err := ledgerObject(append(bytes.Clone(raw), []byte(" {}")...)); err == nil {
			t.Fatal("trailing object accepted")
		}
	})
}
