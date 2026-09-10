package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

func TestBinaryExportWithoutProviderPreservesSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	home, workspace := t.TempDir(), t.TempDir()
	manager, err := sessionio.NewManager(filepath.Join(home, ".hand", "sessions"), workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Create(ctx, "source")
	if err != nil {
		t.Fatal(err)
	}
	if err := sessionio.BeginUsageTracking(first.Session); err != nil {
		t.Fatal(err)
	}
	image := bytes.Repeat([]byte{31}, 2<<20)
	first.Session.Append(session.UserMessageWithImagesEntry("offline export content", []session.ImageData{{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(image)}}))
	reported := llm.RequestUsage{ID: "export-request", Model: "fixture", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 19, OutputTokens: 5}}
	unknown := llm.RequestUsage{ID: "export-unknown", Model: "fixture", Status: "cancelled", Category: llm.CallRetry, Source: "unavailable"}
	for _, request := range []llm.RequestUsage{reported, unknown} {
		if err := sessionio.RecordRequestUsage(first.Session, request); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	first.Session.Close()
	second, err := manager.Create(ctx, "active")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Session.Close()
	// A malformed provider config proves export does not initialise the model path.
	if err := os.WriteFile(filepath.Join(home, ".hand", "config.json"), []byte("invalid model config"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "export with spaces.jsonl")
	invoke := func(path string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, binary, "--session", first.Session.ID, "--export-session", path)
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "HOME="+home, "OPENAI_API_KEY=", "ANTHROPIC_API_KEY=", "GEMINI_API_KEY=")
		return cmd.CombinedOutput()
	}
	if out, err := invoke(destination); err != nil {
		t.Fatalf("offline export: %v: %s", err, out)
	}
	sourcePath, err := manager.SessionPath(first.Record)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := os.ReadFile(destination)
	if err != nil || string(exported) != string(source) {
		t.Fatal("export not byte-identical", err)
	}
	snapshot, err := manager.Catalogue().Snapshot()
	if err != nil || snapshot.LastActiveID != second.Session.ID {
		t.Fatal("export changed active session", err)
	}
	if _, err := invoke(destination); err == nil {
		t.Fatal("binary overwrote destination")
	}
	// Restart the real client with only the exported journal and its sidecars.
	// A separate home prevents the original catalogue from supplying state.
	restartedHome := t.TempDir()
	restartedStore := filepath.Join(restartedHome, ".hand", "sessions", "hand")
	if err := os.MkdirAll(restartedStore, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restartedStore, filepath.Base(sourcePath)), exported, 0600); err != nil {
		t.Fatal(err)
	}
	sidecars := filepath.Join(filepath.Dir(destination), ".attachments")
	if err := os.CopyFS(filepath.Join(restartedStore, ".attachments"), os.DirFS(sidecars)); err != nil {
		t.Fatal(err)
	}
	// CopyFS uses ordinary directory modes; restore the private attachment
	// store modes required by the actual reader before reconciling the import.
	privateBlobs := filepath.Join(restartedStore, ".attachments")
	if err := os.Chmod(privateBlobs, 0700); err != nil {
		t.Fatal(err)
	}
	blobEntries, err := os.ReadDir(privateBlobs)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range blobEntries {
		if err := os.Chmod(filepath.Join(privateBlobs, entry.Name()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(restartedHome, ".hand", "config.json"), []byte("invalid provider config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := second.Session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(home, ".hand", "sessions"), filepath.Join(home, ".hand", "original-offline")); err != nil {
		t.Fatal(err)
	}
	// Explicitly import the copied journal through the same reconciliation API
	// used at startup; export alone deliberately requires a catalogue identity.
	reconciled, err := sessionio.NewManager(filepath.Dir(restartedStore), workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reconciled.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	roundtrip := filepath.Join(t.TempDir(), "roundtrip.jsonl")
	cmd := exec.CommandContext(ctx, binary, "--session", first.Session.ID, "--export-session", roundtrip)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "HOME="+restartedHome, "OPENAI_API_KEY=", "ANTHROPIC_API_KEY=", "GEMINI_API_KEY=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restarted export: %v: %s", err, out)
	}
	roundtripBytes, err := os.ReadFile(roundtrip)
	if err != nil || !bytes.Equal(roundtripBytes, exported) {
		t.Fatalf("restarted export changed journal: %v", err)
	}
	loaded, err := session.NewStore(filepath.Dir(roundtrip)).LoadExclusive("", "roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	usage, err := sessionio.ReadUsage(loaded)
	expected := sessionio.UsageSummary{Total: *reported.Usage, Requests: 2, Unknown: 1}
	if err != nil || usage != expected {
		t.Fatalf("restarted usage got %+v want %+v: %v", usage, expected, err)
	}
	entries, err := loaded.ResolveImages(ctx, loaded.History())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected history: %d", len(entries))
	}
	var message session.MessageData
	if err := json.Unmarshal(entries[0].Data, &message); err != nil {
		t.Fatal(err)
	}
	if len(message.Images) != 1 {
		t.Fatalf("missing exported image: %+v", message)
	}
	restored, err := base64.StdEncoding.DecodeString(message.Images[0].Data)
	if err != nil || !bytes.Equal(restored, image) {
		t.Fatalf("restarted attachment changed: %v", err)
	}
}

func TestExportInvocationRejectsMissingIdentityAndPrompt(t *testing.T) {
	for _, args := range [][]string{{"--export-session=out.jsonl"}, {"--export-session=out.jsonl", "--session=one", "-p=unexpected"}} {
		t.Run(args[0], func(t *testing.T) {
			invocationFixture(t, args...)
			if err := run(); err == nil || exitCode(err) != 2 {
				t.Fatal("invalid export invocation accepted", err)
			}
		})
	}
}
