package tui

import (
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/process"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactViewerAfterSessionReopen(t *testing.T) {
	captures, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "captures"))
	if err != nil {
		t.Fatal(err)
	}
	output := strings.Repeat("full captured line\n", 500) + "final captured marker"
	capture := captures.Capture(4)
	capture.Write([]byte(output))
	if err := capture.Close(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	sess.Append(session.ToolResultWithArtifactsEntry("call", "short preview", "", nil, map[string]process.ArtifactInfo{"stdout": capture.Info()}))
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	controller := &app.Controller{Rt: &runtime.Runtime{Session: sess}, OutputStore: captures}
	m := NewModel(controller, t.TempDir())
	m.SetController(controller)
	defer m.CloseApplication()
	m.LoadHistory(sess.View())
	cmd := m.showOutput("1 stdout")
	if cmd == nil {
		t.Fatal("artifact lookup not requested")
	}
	m.Update(cmd())
	if m.outputView.block.Artifact != "stdout" || m.outputView.block.Output != output {
		t.Fatal("artifact not expanded", m.outputView.status)
	}
	m.outputView.viewport.GotoBottom()
	if !strings.Contains(m.View(), "final captured marker") {
		t.Fatal("capture tail inaccessible")
	}
	if err := os.Remove(capture.Info().Path); err != nil {
		t.Fatal(err)
	}
	cmd = m.showOutput("1 stdout")
	m.Update(cmd())
	if !strings.Contains(m.outputView.status, "unavailable") || m.outputView.block.Output != "short preview" {
		t.Fatal("missing artifact not reported", m.outputView.status)
	}
	cmd = m.showOutput("1 stderr")
	m.Update(cmd())
	if !strings.Contains(m.outputView.status, "no persisted artifact") {
		t.Fatal("missing stream silently accepted", m.outputView.status)
	}
}
