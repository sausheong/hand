package rpc

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzCanonicalControlParams(f *testing.F) {
	for _, seed := range []string{`{}`, `{"limit":1000,"confirmed":true}`, `{"confirmed":false,"confirmed":true}`, `{"confi\u0072med":true}`, `{"Confirmed":true}`, `{"limit":null}`, `{"limit":9223372036854775808,"confirmed":true}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			return
		}
		var value struct {
			Limit     int64 `json:"limit"`
			Confirmed bool  `json:"confirmed"`
		}
		if err := decodeParams(raw, &value); err != nil {
			return
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal("accepted non-object", err)
		}
		for key := range fields {
			if key != "limit" && key != "confirmed" {
				t.Fatal("accepted noncanonical field")
			}
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		copy := value
		if err := decodeParams(canonical, &copy); err != nil || copy != value {
			t.Fatalf("control interpretation changed on round trip: %v", err)
		}
		if _, ok := fields["confirmed"]; ok {
			tail := bytes.TrimSpace(raw)[1:]
			for _, key := range []string{`"confirmed"`, `"confi\u0072med"`} {
				duplicate := append([]byte("{"+key+":false,"), tail...)
				if err := decodeParams(duplicate, &copy); err == nil {
					t.Fatal("duplicate confirmation accepted")
				}
			}
		}
	})
}
