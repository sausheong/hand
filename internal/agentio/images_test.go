package agentio_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
)

func TestExtractImagePaths_ExistingInWorkspaceImageExtracted(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(imgPath, []byte("fake png bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, images := agentio.ExtractImagePaths(dir, "look at "+imgPath+" please")

	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}
	if images[0].MimeType != "image/png" {
		t.Errorf("MimeType = %q, want image/png", images[0].MimeType)
	}
	if string(images[0].Data) != "fake png bytes" {
		t.Errorf("Data = %q, want the file's bytes", images[0].Data)
	}
	if !strings.Contains(text, "[image: shot.png]") {
		t.Errorf("text = %q, want it to contain the placeholder", text)
	}
	if strings.Contains(text, imgPath) {
		t.Errorf("text = %q, want the raw path replaced", text)
	}
}

func TestExtractImagePaths_NonExistentPathLeftAlone(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.png")

	text, images := agentio.ExtractImagePaths(dir, "see "+missing)

	if images != nil {
		t.Fatalf("got %d images, want nil", len(images))
	}
	if !strings.Contains(text, missing) {
		t.Errorf("text = %q, want the raw path left untouched", text)
	}
}

func TestExtractImagePaths_NonImageExtensionLeftAlone(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, images := agentio.ExtractImagePaths(dir, "read "+txtPath)

	if images != nil {
		t.Fatalf("got %d images, want nil", len(images))
	}
	if !strings.Contains(text, txtPath) {
		t.Errorf("text = %q, want the raw path left untouched", text)
	}
}

func TestExtractImagePaths_MultipleImagesAllExtracted(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.png")
	p2 := filepath.Join(dir, "b.jpg")
	if err := os.WriteFile(p1, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, images := agentio.ExtractImagePaths(dir, p1+" and "+p2)

	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}
	if !strings.Contains(text, "[image: a.png]") || !strings.Contains(text, "[image: b.jpg]") {
		t.Errorf("text = %q, want both placeholders", text)
	}
}

func TestExtractImagePaths_NoPathsReturnsUnchanged(t *testing.T) {
	dir := t.TempDir()
	text, images := agentio.ExtractImagePaths(dir, "just a plain message")

	if text != "just a plain message" {
		t.Errorf("text = %q, want it unchanged", text)
	}
	if images != nil {
		t.Fatalf("got %d images, want nil", len(images))
	}
}

func TestExtractImagePaths_ImageOutsideWorkspaceNotAttached(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	imgPath := filepath.Join(outside, "hostname.png")
	if err := os.WriteFile(imgPath, []byte("should not leave the box"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, images := agentio.ExtractImagePaths(workspace, "see "+imgPath)

	if images != nil {
		t.Fatalf("got %d images, want nil (path outside workspace must never be attached)", len(images))
	}
	if !strings.Contains(text, imgPath) {
		t.Errorf("text = %q, want the raw out-of-workspace path left untouched", text)
	}
}
