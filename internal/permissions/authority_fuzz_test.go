package permissions

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzAuthorityJournalJSON(f *testing.F) {
	for _, seed := range []string{
		`{"version":1,"revoke":"grant-1"}`,
		`{"version":1,"grant":{"ID":"grant-1","Operation":"file.write","Scope":"exact","Resource":"/workspace/file","Lifetime":"persistent","SessionID":"","InvocationID":"","Provenance":"user_decision","Workspace":"/workspace","ConfigDigest":"digest"}}`,
		`{"version":1,"version":2,"revoke":"grant-1"}`,
		`{"version":1,"grant":null}`,
		`{"version":1,"revoke":"grant-1"} {}`,
		`{"version":1,"grant":{"ID":"x","\u0049D":"y"}}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) >= MaxAuthorityRecordBytes {
			return
		}
		if validateAuthorityJSON(raw) != nil {
			return
		}
		// Successful structural validation must still be a single valid JSON value.
		if !json.Valid(raw) {
			t.Fatal("invalid JSON accepted")
		}
		var record authorityRecord
		if json.Unmarshal(raw, &record) != nil {
			return
		} // Type validation occurs after structural validation in replay.
		canonical, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if validateAuthorityJSON(canonical) != nil {
			t.Fatal("typed authority could not round-trip", string(canonical))
		}
		var again authorityRecord
		if err = json.Unmarshal(canonical, &again); err != nil {
			t.Fatal(err)
		}
		next, err := json.Marshal(again)
		if err != nil || !bytes.Equal(canonical, next) {
			t.Fatal("unstable authority round-trip", err)
		}
		// Inject duplicate decoded names into otherwise accepted records. Test an
		// escaped spelling too: textual comparison alone cannot detect this alias.
		for _, key := range []string{`"version"`, `"\u0076ersion"`} {
			ambiguous := append([]byte("{"+key+":1,"), bytes.TrimSpace(raw)[1:]...)
			if validateAuthorityJSON(ambiguous) == nil {
				t.Fatal("duplicate version accepted")
			}
		}
	})
}
