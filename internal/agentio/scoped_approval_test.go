package agentio

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/permissions"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type scopedSender func(any)

func (f scopedSender) Send(v any) { f(v) }
func TestScopedHookPersistsOnlyApprovedResource(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	dir := filepath.Join(t.TempDir(), "authority")
	a, err := permissions.OpenAuthority(dir, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	prompts := 0
	hook := NewScopedApprovalHook(scopedSender(func(v any) { r := v.(ApprovalRequest); prompts++; r.Respond <- DecisionAlways }), a, workspace, digest, nil)
	input := json.RawMessage(`{"path":"one.txt","content":"one"}`)
	decision, err := hook(context.Background(), "write_file", input)
	if err != nil || !decision.Allow {
		t.Fatal(decision, err)
	}
	decision, err = hook(context.Background(), "write_file", input)
	if err != nil || !decision.Allow || prompts != 1 {
		t.Fatal("same scope prompted again", err)
	}
	if _, err = hook(context.Background(), "write_file", json.RawMessage(`{"path":"two.txt","content":"two"}`)); err != nil || prompts != 2 {
		t.Fatal("new path reused authority", err)
	}
}
func TestScopedHookRejectsChangedSymlinkDuringApproval(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "inside")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "target")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	hook := NewScopedApprovalHook(scopedSender(func(v any) {
		r := v.(ApprovalRequest)
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		r.Respond <- DecisionOnce
	}), nil, workspace, strings.Repeat("a", 64), nil)
	decision, err := hook(context.Background(), "write_file", json.RawMessage(`{"path":"target/file","content":"data"}`))
	if err == nil || decision.Allow {
		t.Fatal("stale resolved resource approved")
	}
}
func TestScopedHookExternalReadAndUnknownToolRequireDecision(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "data")
	raw, _ := json.Marshal(map[string]string{"path": outside})
	hook := NewScopedApprovalHook(nil, nil, workspace, strings.Repeat("a", 64), nil)
	for _, r := range []struct {
		name string
		raw  json.RawMessage
	}{{"read_file", raw}, {"unknown", json.RawMessage(`{}`)}} {
		d, err := hook(context.Background(), r.name, r.raw)
		if err != nil || d.Allow {
			t.Fatal("unapproved capability allowed", err)
		}
	}
	d, err := hook(context.Background(), "read_file", json.RawMessage(`{"path":"workspace-file"}`))
	if err != nil || !d.Allow {
		t.Fatal("workspace read denied", err)
	}
}

func TestScopedHookRejectsStaleFileContentAndMode(t *testing.T) {
	for _, change := range []string{"content", "mode", "created"} {
		t.Run(change, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "file")
			if change != "created" {
				if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			hook := NewScopedApprovalHook(scopedSender(func(v any) {
				r := v.(ApprovalRequest)
				if !strings.Contains(r.Preview, "Before:") {
					t.Fatal("snapshot absent from preview")
				}
				var err error
				if change == "mode" {
					err = os.Chmod(path, 0644)
				} else {
					err = os.WriteFile(path, []byte("changed"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				r.Respond <- DecisionOnce
			}), nil, workspace, strings.Repeat("a", 64), nil)
			decision, err := hook(context.Background(), "write_file", json.RawMessage(`{"path":"file","content":"replacement"}`))
			if err == nil || decision.Allow {
				t.Fatal("stale file accepted")
			}
		})
	}
}
