package agentio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageInputQuotedEscapedAndRepeatedPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a b.png"), []byte("image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	text := `look at @"a b.png" and a\ b.png please`
	display, images, err := ParseImageInput(context.Background(), dir, text)
	if err != nil || len(images) != 1 || string(images[0].Data) != "image bytes" || display != "look at [image: a b.png] and [image: a b.png] please" {
		t.Fatal(display, images, err)
	}
	plain := "don't interpret ordinary prose"
	if got, images, err := ParseImageInput(context.Background(), "", plain); err != nil || got != plain || len(images) != 0 {
		t.Fatal(got, images, err)
	}
}

func TestImageInputErrorsAreAtomicAndBounded(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.png"), []byte("good"), 0600)
	outside := filepath.Join(t.TempDir(), "outside.png")
	os.WriteFile(outside, []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(dir, "link.png"))
	large := filepath.Join(dir, "large.png")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(maxImageFileSize + 1)
	f.Close()
	for _, path := range []string{"missing.png", "large.png", "link.png", outside, `"unfinished.png`} {
		text := "good.png " + path
		display, images, err := ParseImageInput(context.Background(), dir, text)
		if err == nil || len(images) != 0 || display != text {
			t.Fatal("partial attachment success", path, display, len(images), err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, images, err := ParseImageInput(ctx, dir, "good.png"); err == nil || images != nil {
		t.Fatal("cancelled input loaded")
	}
}

func TestImageInputAggregateLimit(t *testing.T) {
	dir := t.TempDir()
	var names []string
	for _, name := range []string{"a.png", "b.png", "c.png", "d.png", "e.png"} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		f.Truncate(maxImageFileSize)
		f.Close()
		names = append(names, name)
	}
	if _, images, err := ParseImageInput(context.Background(), dir, strings.Join(names, " ")); err == nil || images != nil {
		t.Fatal("aggregate limit bypassed")
	}
}
