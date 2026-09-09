package tui

import (
	"bytes"
	"encoding/base64"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/session"
	"io"
	"strings"
	"testing"
)

func TestFullOutputLiveReplayAndScrolling(t *testing.T) {
	raw := strings.Repeat("line of full output\n", 100) + "last marker\x1b]52;c;payload\a"
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	m.renderApplicationEvent(app.Event{Kind: "tool_result", Details: app.Details{ToolID: "tool", ResultPresent: true, Output: raw}})
	if len(m.toolOutputs) != 1 || m.toolOutputs[0].Output != raw {
		t.Fatal("full output discarded")
	}
	replay := NewModel(nil, t.TempDir())
	replay.LoadHistory([]session.SessionEntry{session.ToolResultEntry("tool", raw, "", nil)})
	if replay.toolOutputs[0].Output != m.toolOutputs[0].Output || replay.toolOutputs[0].Error != m.toolOutputs[0].Error || replay.toolOutputs[0].ToolID != m.toolOutputs[0].ToolID || replay.transcript[0] != m.transcript[0] {
		t.Fatal("live/replay mismatch")
	}
	m.showOutput("")
	if m.outputView == nil || m.outputView.viewport.Height > 22 {
		t.Fatal("viewer not bounded")
	}
	if strings.Contains(m.View(), "last marker") {
		t.Fatal("unexpected bottom at open")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "last marker") || strings.Contains(m.View(), "payload") {
		t.Fatal("full output inaccessible or unsanitised")
	}
	m.resize(40, 12)
	m.resizeOutput()
	if m.outputView.viewport.Height != 10 || m.outputView.viewport.Width != 40 {
		t.Fatal("resize ignored")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.outputView != nil {
		t.Fatal("viewer did not close")
	}
}

func TestOutputKeepsFailureContent(t *testing.T) {
	b := ToolOutput{Output: "partial stdout", Error: "failure\x1b[2J"}
	if !strings.Contains(b.text(), "partial stdout") || !strings.Contains(b.text(), "failure") || strings.Contains(b.text(), "\x1b") {
		t.Fatal(b.text())
	}
}

func TestOutputSearchAcrossWrapAndInputKeys(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.toolOutputs = []ToolOutput{{Output: strings.Repeat("before\n", 40) + strings.Repeat("long", 20) + "needle" + strings.Repeat("after\n", 30) + "needle again"}}
	m.showOutput("")
	m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("needle")})
	m.outputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.outputView.matches) != 2 || m.outputView.viewport.YOffset == 0 {
		t.Fatal("search did not navigate")
	}
	if !strings.Contains(m.View(), "needle") {
		t.Fatal("match is outside visible wrapped rows")
	}
	first := m.outputView.viewport.YOffset
	m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.outputView.viewport.YOffset <= first {
		t.Fatal("next did not advance")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	if m.outputView.viewport.YOffset != first {
		t.Fatal("previous did not return")
	}
	m.resize(40, 24)
	m.resizeOutput()
	if len(m.outputView.matches) != 2 {
		t.Fatal("resize lost matches")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.outputView.draft = "missing"
	m.outputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.outputView.status != "0 matching lines" {
		t.Fatal(m.outputView.status)
	}
}

type shortClipboardWriter struct{}

func (shortClipboardWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestOutputCopyEncodesAndReportsWriteErrors(t *testing.T) {
	var out bytes.Buffer
	c := clipboardWrite{text: "full \x1b]52;c;untrusted\a output", out: &out}
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(out.String(), "\x1b]52;c;"), "\a")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(data) != c.text {
		t.Fatal("copy was truncated or interpolated")
	}
	c.out = shortClipboardWriter{}
	if c.Run() != io.ErrShortWrite {
		t.Fatal("short write reported success")
	}
	m := NewModel(nil, t.TempDir())
	m.toolOutputs = []ToolOutput{{Output: "text"}}
	m.showOutput("")
	_, cmd := m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("copy command missing")
	}
	old := m.outputView
	m.showOutput("")
	m.Update(outputCopyResult{viewer: old, err: io.ErrClosedPipe})
	if m.outputView.status != "" {
		t.Fatal("stale copy result changed new viewer")
	}
	m.Update(outputCopyResult{viewer: m.outputView, err: io.ErrClosedPipe})
	if !strings.Contains(m.outputView.status, "Copy failed") {
		t.Fatal("copy failure hidden")
	}
}
