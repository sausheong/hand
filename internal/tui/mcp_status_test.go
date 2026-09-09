package tui

import (
	"github.com/sausheong/harness/runtime"
	"strings"
	"testing"
)

func TestMCPStartupStatusVisible(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.SetMCPStatus([]runtime.MCPServerStatus{{Name: "optional", Optional: true, State: "unavailable"}, {Name: "healthy", State: "connected", Tools: 2}})
	text := strings.Join(m.transcript, "\n")
	for _, want := range []string{"optional: unavailable", "[optional]", "healthy: connected (2 tools)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
}
