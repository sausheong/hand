package sessionio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

func TestAttachmentForkExportAndRestart(t *testing.T) {
	ctx := context.Background()
	manager, err := NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	source, err := manager.Create(ctx, "images")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Session.Close()
	if err := BeginUsageTracking(source.Session); err != nil {
		t.Fatal(err)
	}
	reported := llm.RequestUsage{ID: "image-request", Model: "fixture", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 37, OutputTokens: 11, CacheReadInputTokens: 7}}
	unknown := llm.RequestUsage{ID: "interrupted-request", Model: "fixture", Status: "cancelled", Category: llm.CallRetry, Source: "unavailable"}
	for _, request := range []llm.RequestUsage{reported, unknown} {
		if err := RecordRequestUsage(source.Session, request); err != nil {
			t.Fatal(err)
		}
	}
	expectedUsage := UsageSummary{Total: *reported.Usage, Requests: 2, Unknown: 1}
	checkUsage := func(sess *session.Session, expected UsageSummary) {
		t.Helper()
		got, err := ReadUsage(sess)
		if err != nil || got != expected {
			t.Fatalf("usage got %+v want %+v: %v", got, expected, err)
		}
	}
	checkUsage(source.Session, expectedUsage)
	payload := bytes.Repeat([]byte{42}, 12<<20)
	source.Session.Append(session.UserMessageWithImagesEntry("image", []session.ImageData{{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(payload)}}))
	if err := source.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	fork, err := manager.Fork(ctx, source.Session.History())
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Session.Close()
	check := func(sess *session.Session) {
		t.Helper()
		entries, err := sess.ResolveImages(ctx, sess.History())
		if err != nil {
			t.Fatal(err)
		}
		var message session.MessageData
		if err := json.Unmarshal(entries[0].Data, &message); err != nil {
			t.Fatal(err)
		}
		data, err := base64.StdEncoding.DecodeString(message.Images[0].Data)
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatal("image changed", err)
		}
	}
	check(fork.Session)
	checkUsage(fork.Session, UsageSummary{})
	blobs, err := manager.store.OpenAttachments("hand")
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := blobs.Put(ctx, []byte("unreferenced"))
	blobs.Close()
	if err != nil {
		t.Fatal(err)
	}
	destinationDir := t.TempDir()
	destination := filepath.Join(destinationDir, "export.jsonl")
	if err := manager.Export(ctx, source.Session, source.Record.StoreKey, destination); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(destination)
	if err != nil || len(raw) > 8192 {
		t.Fatal("export embeds large image", err)
	}
	if _, err := os.Stat(filepath.Join(destinationDir, ".attachments", unrelated.SHA256)); !os.IsNotExist(err) {
		t.Fatal("unreferenced blob exported", err)
	}
	// Remove the original blob store to prove the exported session is independent.
	if err := os.Rename(filepath.Join(manager.root, "hand", ".attachments"), filepath.Join(manager.root, "original-blobs")); err != nil {
		t.Fatal(err)
	}
	imported, err := session.NewStore(destinationDir).LoadExclusive("", "export")
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	check(imported)
	checkUsage(imported, expectedUsage)
	// Replaying an already recorded request after standalone import must not
	// double-charge, and continuing the imported session preserves old unknowns.
	if err := RecordRequestUsage(imported, reported); err != nil {
		t.Fatal(err)
	}
	checkUsage(imported, expectedUsage)
	continued := llm.RequestUsage{ID: "after-import", Model: "fixture", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 4, OutputTokens: 2}}
	if err := RecordRequestUsage(imported, continued); err != nil {
		t.Fatal(err)
	}
	if err := imported.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := session.NewStore(destinationDir).LoadExclusive("", "export")
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	expectedUsage.Requests++
	expectedUsage.Total.InputTokens += 4
	expectedUsage.Total.OutputTokens += 2
	checkUsage(restarted, expectedUsage)
	check(restarted)
}

func TestAttachmentExportMissingBlobDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	manager, err := NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	source, err := manager.Create(ctx, "images")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Session.Close()
	source.Session.Append(session.UserMessageWithImagesEntry("image", []session.ImageData{{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image"))}}))
	refs, err := session.AttachmentReferences(source.Session.Entries())
	if err != nil || len(refs) != 1 {
		t.Fatal(refs, err)
	}
	if err := os.Remove(filepath.Join(manager.root, "hand", ".attachments", refs[0].SHA256)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "export.jsonl")
	if err := manager.Export(ctx, source.Session, source.Record.StoreKey, destination); err == nil {
		t.Fatal("missing attachment export succeeded")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("broken export published", err)
	}
	if _, err := manager.Fork(ctx, source.Session.History()); err == nil {
		t.Fatal("fork succeeded without image")
	}
}
