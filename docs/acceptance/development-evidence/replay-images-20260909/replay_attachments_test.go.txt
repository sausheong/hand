package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/session"
)

func TestReplayedAttachmentsVisibleAfterRestart(t *testing.T) {
	ctx := context.Background()
	root, workspace := t.TempDir(), t.TempDir()
	manager, err := sessionio.NewManager(root, workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Create(ctx, "images")
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte("private image bytes must never appear in transcript"))
	img := session.ImageData{MimeType: "image/png", Data: payload}
	current.Session.Append(session.UserMessageWithImagesEntry("inspect this", []session.ImageData{img}))
	current.Session.Append(session.ToolResultEntry("image-tool", "generated", "", []session.ImageData{img, img}))
	if err := current.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := current.Session.Close(); err != nil {
		t.Fatal(err)
	}
	manager, err = sessionio.NewManager(root, workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Open(ctx, current.Record.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Session.Close()
	entries := reopened.Session.History()
	if len(entries) != 2 {
		t.Fatal("history changed", len(entries))
	}
	var stored session.MessageData
	if err := json.Unmarshal(entries[0].Data, &stored); err != nil || len(stored.Images) != 1 || stored.Images[0].Reference == nil {
		t.Fatal("fixture did not externalize attachment", err)
	}
	assertVisible := func(lines []string) {
		t.Helper()
		if len(lines) != 2 || !strings.Contains(lines[0], "inspect this") || !strings.Contains(lines[0], "[1 image attachment]") || !strings.Contains(lines[1], "[2 image attachments]") {
			t.Fatal("attachments invisible", lines)
		}
		if strings.Contains(strings.Join(lines, "\n"), payload) || strings.Contains(strings.Join(lines, "\n"), stored.Images[0].Reference.SHA256) {
			t.Fatal("attachment contents or storage identity exposed")
		}
	}
	assertVisible(ReplayHistory(entries, 80, "dark"))
	m := NewModel(nil, workspace)
	m.LoadHistory(entries)
	assertVisible(m.transcript)
	m.resize(32, 24)
	assertVisible(m.transcript)
	// Legacy inline images use the same metadata-only projection.
	assertVisible(ReplayHistory([]session.SessionEntry{session.UserMessageWithImagesEntry("inspect this", []session.ImageData{img}), session.ToolResultEntry("legacy", "generated", "", []session.ImageData{img, img})}, 80, "dark"))
}
