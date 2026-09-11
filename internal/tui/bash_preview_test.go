package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/session"
)

func TestBashPreviewFiveLinesAndWidth(t *testing.T) {
	command := "git status\ngit diff\ngit add main.go\ngit commit -m 'Fix'\ngit log -1\nsecret sixth line"
	raw, _ := json.Marshal(map[string]string{"command": command})
	got := bashCommandPreview(string(raw), 80)
	if !strings.Contains(got, "git log -1") || strings.Contains(got, "secret sixth") || !strings.Contains(got, "more lines") {
		t.Fatal(got)
	}
	raw, _ = json.Marshal(map[string]string{"command": strings.Repeat("界", 80) + "\n\x1b[2Jecho safe"})
	got = bashCommandPreview(string(raw), 30)
	for _, line := range strings.Split(got, "\n") {
		if ansi.StringWidth(line) > 30 {
			t.Fatalf("too wide %q", line)
		}
	}
	if strings.Contains(got, "\x1b") || !strings.Contains(got, "echo safe") {
		t.Fatal(got)
	}
	if bashCommandPreview(`{"command":`, 80) != "" {
		t.Fatal("malformed JSON rendered")
	}
}
func TestReadyCommandUpdatesHeaderWithoutDuplicate(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	defer m.CloseApplication()
	start := app.Event{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: "bash", ToolID: "one"}}
	m.renderApplicationEvent(start)
	ready := start
	ready.Kind = "tool_call_ready"
	ready.Details.ToolInput = `{"command":"git status\ngit diff"}`
	m.renderApplicationEvent(ready)
	m.renderApplicationEvent(ready)
	if len(m.transcript) != 1 || m.toolCallsThisTurn != 1 || !strings.Contains(m.transcript[0], "git diff") {
		t.Fatalf("%v calls=%d", m.transcript, m.toolCallsThisTurn)
	}
	ready.Details.ToolID = "two"
	m.renderApplicationEvent(ready)
	if len(m.transcript) != 2 || m.toolCallsThisTurn != 2 {
		t.Fatal("missing start not handled")
	}
	raw, _ := json.Marshal(session.ToolCallData{Tool: "bash", Input: json.RawMessage(ready.Details.ToolInput)})
	block, ok := sessionBlock(session.SessionEntry{Type: session.EntryTypeToolCall, Data: raw})
	if !ok || !strings.Contains(block.render(80, ""), "git diff") {
		t.Fatal("replay lost preview")
	}
}
