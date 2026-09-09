package sessionio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerRecoversLegacyClonesAndTail(t *testing.T) {
	m := managerFixture(t)
	key := m.legacyKeys[0]
	path := filepath.Join(m.root, m.agent, key+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"a","type":"message","data":{"text":"root"}}` + "\n" +
		`{"id":"b","parentId":"a","type":"message","data":{"text":"kept"}}` + "\n" +
		`{"id":"summary","parentId":"a","type":"compaction","data":{"summary":"root summary"}}` + "\n" +
		`{"id":"b","parentId":"summary","type":"message","data":{"text":"kept"}}` + "\n" +
		`{"id":"unfinished"`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := m.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Backups) != 1 || len(selected.Session.Entries()) != 4 {
		t.Fatal("combined repair missing", selected.Backups)
	}
	backup, err := os.ReadFile(selected.Backups[0])
	if err != nil || string(backup) != raw {
		t.Fatal("original backup changed", err)
	}
	if err := selected.Session.Branch("b"); err != nil {
		t.Fatal(err)
	}
	id := selected.Session.ID
	selected.Session.Close()
	resumed, err := m.Open(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Session.Close()
	if len(resumed.Backups) != 0 || resumed.Session.LeafID() != "b" || len(resumed.Session.History()) != 2 {
		t.Fatal("recovery was not idempotent", resumed.Backups)
	}
}
