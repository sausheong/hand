package agentio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAttachmentPolicyExactSnapshot(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	file := filepath.Join(external, "selected notes.txt")
	image := filepath.Join(external, "selected image.png")
	other := filepath.Join(external, "other.txt")
	for path, data := range map[string]string{file: "original notes", image: "original image", other: "secret"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	policy, err := NewAttachmentPolicy(ctx, []string{file, image})
	if err != nil {
		t.Fatal(err)
	}
	ctx = WithAttachmentPolicy(ctx, policy)
	if err := os.WriteFile(file, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(image); err != nil {
		t.Fatal(err)
	}
	input, err := ParsePromptInput(ctx, workspace, "@\""+file+"\" @\""+image+"\"")
	if err != nil || !strings.Contains(input.Prompt, "original notes") || len(input.Images) != 1 || string(input.Images[0].Data) != "original image" {
		t.Fatalf("snapshot: %#v %v", input, err)
	}
	input.Images[0].Data[0] = 'X'
	retry, err := ParsePromptInput(ctx, workspace, "@\""+image+"\"")
	if err != nil || string(retry.Images[0].Data) != "original image" {
		t.Fatalf("mutable grant: %#v %v", retry, err)
	}
	for _, prompt := range []string{"@" + other, "\"" + image + "\""} {
		if _, err := ParsePromptInput(ctx, workspace, prompt); err == nil {
			t.Fatalf("unauthorised reference accepted: %s", prompt)
		}
	}
	if _, err := ParsePromptInput(context.Background(), workspace, "@\""+file+"\""); err == nil {
		t.Fatal("grant leaked across contexts")
	}
}

func TestAttachmentPolicyLimitsAndSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAttachmentPolicy(context.Background(), []string{fifo}); err == nil {
		t.Fatal("FIFO accepted")
	}
	if _, err := NewAttachmentPolicy(context.Background(), []string{dir}); err == nil {
		t.Fatal("directory accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewAttachmentPolicy(ctx, []string{fifo}); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, make([]byte, MaxReferenceBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := NewAttachmentPolicy(context.Background(), []string{large})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePromptInput(WithAttachmentPolicy(context.Background(), p), t.TempDir(), "@"+large); err == nil {
		t.Fatal("text size limit bypassed")
	}
}
