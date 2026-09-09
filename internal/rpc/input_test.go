//go:build darwin || linux

package rpc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
)

func TestRPCPromptResolvesWorkspaceReference(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("unique attached content"), 0600); err != nil {
		t.Fatal(err)
	}
	backend := &queuedBackend{prompts: make(chan string, 1)}
	s := app.New(backend, app.Options{SessionID: "session", MaxIterations: 1, ResolveInput: func(ctx context.Context, text string) (agentio.PromptInput, error) {
		return agentio.ParsePromptInput(ctx, workspace, text)
	}})
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	d := NewDispatcher(s, l)
	defer d.Close()
	d.Dispatch(context.Background(), rpcRequest("hello", "hello", `{}`))
	if r := d.Dispatch(context.Background(), rpcRequest("p1", "prompt", `{"text":"read @note.txt"}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	select {
	case prompt := <-backend.prompts:
		if !strings.Contains(prompt, "unique attached content") {
			t.Fatalf("reference not resolved: %q", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt not delivered")
	}
	completedRequest(t, l)
}
