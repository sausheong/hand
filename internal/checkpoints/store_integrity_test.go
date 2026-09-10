package checkpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoredSnapshotCorruptionCannotBeLoadedOrSilentlyReplaced(t *testing.T) {
	cases := []struct {
		name, diagnostic string
		edit             func(*storedSnapshot)
	}{
		{"future-version", "identity mismatch", func(s *storedSnapshot) { s.Version++ }},
		{"substituted-identity", "identity mismatch", func(s *storedSnapshot) { s.Digest = strings.Repeat("0", 64) }},
		{"path-traversal", "invalid checkpoint record", func(s *storedSnapshot) { s.Records[0].Path = "../escape" }},
		{"duplicate-record", "invalid checkpoint record", func(s *storedSnapshot) { s.Records = append(s.Records, s.Records[0]) }},
		{"special-mode", "invalid checkpoint record", func(s *storedSnapshot) { s.Records[0].Mode = 04755 }},
		{"unsupported-kind", "invalid checkpoint record", func(s *storedSnapshot) { s.Records[0].Kind = "symlink" }},
		{"missing-content", "content size mismatch", func(s *storedSnapshot) { s.Content = nil }},
		{"incorrect-size", "content size mismatch", func(s *storedSnapshot) { s.Records[0].Size++ }},
		{"unreferenced-content", "unreferenced checkpoint content", func(s *storedSnapshot) { s.Content[strings.Repeat("0", 64)] = []byte("unreferenced") }},
		{"overlapping-omission", "invalid checkpoint omission", func(s *storedSnapshot) { s.Omissions = []Omission{{Path: "file", Reason: "excluded"}} }},
		{"unknown-omission", "invalid checkpoint omission", func(s *storedSnapshot) { s.Omissions = []Omission{{Path: "other", Reason: "not read"}} }},
		{"unsafe-exclusion", "invalid checkpoint exclusion", func(s *storedSnapshot) { s.Exclusions = append(s.Exclusions, "../escape") }},
		{"altered-record-metadata", "digest mismatch", func(s *storedSnapshot) { s.Records[0].Mode = 0644 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			work := t.TempDir()
			put(t, work, "file", "user bytes", 0600)
			snapshot := capture(t, work)
			store, dir := newStore(t, work, DefaultStoreLimits())
			if err := store.Save(ctx, snapshot); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, snapshot.Digest()+".json")
			valid, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var disk storedSnapshot
			if err = json.Unmarshal(valid, &disk); err != nil {
				t.Fatal(err)
			}
			tc.edit(&disk)
			bad, err := json.Marshal(disk)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(file, bad, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = store.Load(ctx, snapshot.Digest()); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatal("corruption not rejected at the expected boundary", err)
			}
			if err = store.Save(ctx, snapshot); err == nil {
				t.Fatal("save silently replaced corrupt evidence")
			}
			got, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(got, bad) {
				t.Fatal("corrupt evidence was rewritten", err)
			}
			if current := capture(t, work); current.Digest() != snapshot.Digest() {
				t.Fatal("snapshot failure changed user files")
			}
			if err = os.WriteFile(file, valid, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load(ctx, snapshot.Digest())
			if err != nil || loaded.Digest() != snapshot.Digest() {
				t.Fatal("authentic snapshot no longer loadable", err)
			}
		})
	}
}
