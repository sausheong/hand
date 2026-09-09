package sessionio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/harness/session"
)

func TestManagerImportsOversizedLegacyImage(t *testing.T) {
	m := managerFixture(t)
	key := m.legacyKeys[0]
	path := filepath.Join(m.root, m.agent, key+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	pixels := bytes.Repeat([]byte{23}, 12<<20)
	entry := session.UserMessageWithImagesEntry("legacy image", []session.ImageData{{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(pixels)}})
	entry.ID = "legacy-image"
	entry.Timestamp = 123
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := m.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Backups) != 1 {
		t.Fatal("migration backup not surfaced", selected.Backups)
	}
	backup, err := os.ReadFile(selected.Backups[0])
	if err != nil || !bytes.Equal(backup, raw) {
		t.Fatal("original backup changed", err)
	}
	id := selected.Session.ID
	selected.Session.Close()
	resumed, err := m.Open(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Session.Close()
	if len(resumed.Backups) != 0 || resumed.Session.LeafID() != entry.ID {
		t.Fatal("repeated import changed identity", resumed.Backups)
	}
	history, err := resumed.Session.ResolveImages(context.Background(), resumed.Session.History())
	if err != nil {
		t.Fatal(err)
	}
	var message session.MessageData
	if err := json.Unmarshal(history[0].Data, &message); err != nil {
		t.Fatal(err)
	}
	restored, err := base64.StdEncoding.DecodeString(message.Images[0].Data)
	if err != nil || !bytes.Equal(restored, pixels) {
		t.Fatal("imported pixels changed", err)
	}
	usage, err := ReadUsage(resumed.Session)
	if err != nil || !usage.PriorUsageUnknown {
		t.Fatal("legacy usage silently became known", usage, err)
	}
}
