package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/app"
)

func TestTUIFileReferenceReachesBackendAsSnapshot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note file.txt"), []byte("reference contents"), 0600)
	var prompt string
	backend := applicationBackend{run: func(_ context.Context, text string) (<-chan app.BackendEvent, error) {
		prompt = text
		ch := make(chan app.BackendEvent, 1)
		ch <- app.BackendEvent{Done: true}
		close(ch)
		return ch, nil
	}}
	m := applicationModel(t, app.New(backend, app.Options{MaxIterations: 1}), dir)
	driveApplication(t, m, m.startRun(`review @"note file.txt"`))
	if !strings.Contains(prompt, "reference contents") || !strings.Contains(prompt, `review @"note file.txt"`) {
		t.Fatal(prompt)
	}
}
