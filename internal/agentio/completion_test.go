package agentio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReferenceCompletionQuotesAndContainsPaths(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "space dir"), 0700)
	os.WriteFile(filepath.Join(dir, "space dir", "note file.txt"), []byte("x"), 0600)
	got, err := CompleteReference(context.Background(), dir, "see @spa", 8)
	if err != nil || !reflect.DeepEqual(got.Candidates, []string{`@"space dir/`}) {
		t.Fatal(got, err)
	}
	text := `see @"space dir/no`
	got, err = CompleteReference(context.Background(), dir, text, len(text))
	if err != nil || !reflect.DeepEqual(got.Candidates, []string{`@"space dir/note file.txt"`}) {
		t.Fatal(got, err)
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "private.txt"), []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(dir, "escape"))
	if _, err := CompleteReference(context.Background(), dir, "@escape/", 8); err == nil {
		t.Fatal("external directory completed")
	}
	if _, err := CompleteReference(context.Background(), dir, "@../", 4); err == nil {
		t.Fatal("traversal completed")
	}
}

func TestReferenceCompletionCursorRangeAndLimits(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "alpha.txt"), nil, 0600)
	text := "before @alOLD after"
	result, err := CompleteReference(context.Background(), dir, text, len("before @al"))
	if err != nil || result.Start != 7 || result.End != len("before @alOLD") || len(result.Candidates) != 1 {
		t.Fatal(result, err)
	}
	for i := 0; i < 65; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("many-%02d", i)), nil, 0600)
	}
	if _, err := CompleteReference(context.Background(), dir, "@many", 5); err == nil {
		t.Fatal("unbounded suggestions")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CompleteReference(ctx, dir, "@", 1); err == nil {
		t.Fatal("cancelled scan")
	}
}
