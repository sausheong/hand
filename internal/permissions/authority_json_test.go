//go:build darwin || linux

package permissions

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthorityRejectsAmbiguousJournalFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func([]byte) []byte
	}{
		{"duplicate-version", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"version":1`), []byte(`"version":2,"version":1`), 1)
		}},
		{"version-alias", func(b []byte) []byte { return bytes.Replace(b, []byte(`"version":1`), []byte(`"Version":1`), 1) }},
		{"scope-escalation", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"Scope":"tree"`), []byte(`"Scope":"exact","Scope":"tree"`), 1)
		}},
		{"escaped-scope-duplicate", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"Scope":"tree"`), []byte(`"Scope":"exact","\u0053cope":"tree"`), 1)
		}},
		{"scope-alias", func(b []byte) []byte { return bytes.Replace(b, []byte(`"Scope":"tree"`), []byte(`"scope":"tree"`), 1) }},
		{"null-session", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"SessionID":""`), []byte(`"SessionID":null`), 1)
		}},
		{"missing-session", func(b []byte) []byte { return bytes.Replace(b, []byte(`"SessionID":"",`), nil, 1) }},
		{"null-revoke", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1,"revoke":null`), 1)
		}},
		{"unknown-field", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1,"future":true`), 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx := scopedFixture(t)
			dir := authorityFixtureDir(t)
			a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.Grant(persistentFixture(ctx)); err != nil {
				t.Fatal(err)
			}
			if err = a.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "grants.jsonl")
			valid, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bad := tc.edit(valid)
			if bytes.Equal(valid, bad) {
				t.Fatal("fixture mutation did not apply")
			}
			if err = os.WriteFile(path, bad, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if got != nil {
				got.Close()
			}
			if err == nil || got != nil {
				t.Fatal("ambiguous journal published authority")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, bad) {
				t.Fatal("rejection modified evidence", err)
			}
			if err = os.WriteFile(path, valid, 0600); err != nil {
				t.Fatal(err)
			}
			a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal("rejection leaked lock", err)
			}
			defer a.Close()
			if !a.Allowed(ctx, AccessRequest{FileWrite, "new.txt"}) {
				t.Fatal("valid journal no longer authorises")
			}
		})
	}
}
