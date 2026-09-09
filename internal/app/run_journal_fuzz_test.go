package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func FuzzRunJournalIntegrity(f *testing.F) {
	for _, raw := range []string{
		`{"version":1,"id":"run","run_id":1,"phase":"start","at":"2026-09-09T00:00:00Z","model":{}}`,
		`{"version":1,"id":"run","run_id":1,"phase":"finish","at":"2026-09-09T00:00:00Z","model":{},"outcome":{"status":"completed","reason":"answer_completed","iterations":1,"verified":false}}`,
		`{"version":1,"version":2}`, `{"Version":1}`, `null`, `[]`, `{} {}`, "\xff",
	} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			t.Skip()
		}
		var record RunRecord
		if decodeRunRecord(raw, &record) != nil || validateRunRecord(record) != nil {
			return
		}
		canonical, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		var roundtrip RunRecord
		if err := decodeRunRecord(canonical, &roundtrip); err != nil {
			t.Fatal("accepted record cannot roundtrip", err)
		}
		if !reflect.DeepEqual(record, roundtrip) {
			t.Fatal("roundtrip changed durable record")
		}
		// encoding/json's ordinary last-field-wins behavior would accept this.
		duplicate := append([]byte(`{"version":2,`), canonical[1:]...)
		if decodeRunRecord(duplicate, new(RunRecord)) == nil {
			t.Fatal("duplicate identity version accepted")
		}
		if record.Outcome != nil {
			duplicate = bytes.Replace(canonical, []byte(`"outcome":{`), []byte(`"outcome":{"status":"cancelled",`), 1)
			if decodeRunRecord(duplicate, new(RunRecord)) == nil {
				t.Fatal("duplicate outcome status accepted")
			}
		}
		if decodeRunRecord(append(canonical, []byte(` {}`)...), new(RunRecord)) == nil {
			t.Fatal("trailing record accepted")
		}
	})
}
