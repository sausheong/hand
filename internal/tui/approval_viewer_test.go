package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"strings"
	"testing"
	"time"
)

func TestApprovalViewerBoundedAndDoesNotAnswer(t *testing.T) {
	m, cmd, returned, release := pendingServiceApproval(t)
	m.resize(80, 24)
	m.pending.Preview = "Path: source.go\n" + strings.Repeat("+ new line\n", 200) + "last diff line\x1b[2J"
	panel := m.approvalPanel()
	if strings.Count(panel, "\n")+1 > 8 || m.bottomHeight() > 8 || !strings.Contains(panel, "[v]") {
		t.Fatalf("unbounded panel: %s", panel)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.outputView == nil || !m.outputView.approval {
		t.Fatal("viewer did not open")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "last diff line") || strings.Contains(m.View(), "\x1b[2J") {
		t.Fatal("full diff inaccessible or unsafe")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if len(returned) != 0 || m.pending == nil {
		t.Fatal("viewer key answered approval")
	}
	m.outputKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pending == nil || len(returned) != 0 {
		t.Fatal("closing viewer answered approval")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	select {
	case ctx := <-returned:
		if ctx.Err() != nil {
			t.Fatal("approval cancelled instead of allowed")
		}
	case <-time.After(time.Second):
		t.Fatal("approval unavailable after viewer")
	}
	release()
	driveApplication(t, m, cmd)
	if m.lastOutcome == nil || m.lastOutcome.Status != agentio.Completed || m.pending != nil {
		t.Fatal("approval did not finish", m.lastOutcome)
	}
}

func TestApprovalLongToolKeepsDecisionControls(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.resize(80, 24)
	m.pending = &agentio.ApprovalRequest{Tool: strings.Repeat("longtool", 50), Preview: "preview"}
	panel := m.approvalPanel()
	if !strings.Contains(panel, "[y]es") || !strings.Contains(panel, "[v]iew") {
		t.Fatal("long tool hides decisions")
	}
}
