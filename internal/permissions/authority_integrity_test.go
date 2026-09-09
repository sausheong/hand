//go:build darwin || linux

package permissions

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthorityCorruptReplayNeverPublishesPartialGrants(t *testing.T) {
	cases := []struct {
		name string
		edit func(*authorityRecord)
	}{
		{"future-version", func(r *authorityRecord) { r.Version++ }},
		{"grant-and-revoke", func(r *authorityRecord) { r.Revoke = r.Grant.ID }},
		{"unknown-revocation", func(r *authorityRecord) { r.Grant = nil; r.Revoke = "missing" }},
		{"ephemeral-grant-on-disk", func(r *authorityRecord) { r.Grant.Lifetime = InvocationGrant }},
		{"project-provenance", func(r *authorityRecord) { r.Grant.Provenance = "project_file" }},
		{"foreign-workspace", func(r *authorityRecord) { r.Grant.Workspace += "-other" }},
		{"foreign-configuration", func(r *authorityRecord) { r.Grant.ConfigDigest = strings.Repeat("b", 64) }},
		{"lifetime-substitution", func(r *authorityRecord) { r.Grant.SessionID = "session" }},
		{"relative-resource", func(r *authorityRecord) { r.Grant.Resource = "new.txt" }},
		{"noncanonical-resource", func(r *authorityRecord) { r.Grant.Resource += "/../other" }},
		{"broad-tool-without-acknowledgement", func(r *authorityRecord) { r.Grant.Operation = LegacyTool; r.Grant.Scope = ExactScope }},
		{"nonfilesystem-tree", func(r *authorityRecord) { r.Grant.Operation = ShellExec }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx := scopedFixture(t)
			dir := authorityFixtureDir(t)
			a, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal(err)
			}
			g := persistentFixture(ctx)
			if err = a.Grant(g); err != nil {
				t.Fatal(err)
			}
			if err = a.Close(); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(dir, "grants.jsonl")
			valid, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			var record authorityRecord
			if err = json.Unmarshal(valid, &record); err != nil {
				t.Fatal(err)
			}
			record.Grant.ID = "second"
			tc.edit(&record)
			bad, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			// A valid first grant must not become accessible when replay of
			// the following record fails.
			bad = append(append(append([]byte(nil), valid...), bad...), '\n')
			if err = os.WriteFile(journal, bad, 0600); err != nil {
				t.Fatal(err)
			}
			if got, err := OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest); err == nil || got != nil {
				if got != nil {
					got.Close()
				}
				t.Fatal("partial authority published from corrupt journal", err)
			}
			got, err := os.ReadFile(journal)
			if err != nil || !bytes.Equal(got, bad) {
				t.Fatal("corrupt journal evidence modified", err)
			}
			if err = os.WriteFile(journal, valid, 0600); err != nil {
				t.Fatal(err)
			}
			a, err = OpenAuthority(dir, ctx.Workspace, ctx.ConfigDigest)
			if err != nil {
				t.Fatal("failed replay leaked ownership", err)
			}
			defer a.Close()
			if !a.Allowed(ctx, AccessRequest{FileWrite, "new.txt"}) || len(a.Grants()) != 1 {
				t.Fatal("authentic grant lost after repairing injected corruption")
			}
		})
	}
}
