package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sausheong/harness/tool/skills"
	"github.com/sausheong/harness/tool/skills/disk"
)

func TestBinarySkillMutationRequiresExplicitApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, action := range []string{"create", "patch", "replace", "remove"} {
		t.Run(action, func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			root := filepath.Join(workspace, ".hand", "skills")
			store := disk.NewStore(root)
			if _, err := store.Create(ctx, skills.Skill{Name: "existing", Body: "original body"}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "existing", "SKILL.md")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			name := "existing"
			if action == "create" {
				name = "new-skill"
			}
			args, _ := json.Marshal(map[string]string{"action": action, "name": name, "body": "replacement body", "old_string": "original", "new_string": "changed"})
			var calls atomic.Int32
			requests := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				requests <- string(body)
				n := calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if n%2 == 1 {
					delta := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "mutation", "type": "function", "function": map[string]any{"name": "skill_manage", "arguments": string(args)}}}}, "finish_reason": nil}}}
					raw, _ := json.Marshal(delta)
					fmt.Fprintf(w, "data: %s\n\n", raw)
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture finished\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			invoke := func(approve bool) {
				t.Helper()
				flags := []string{"--model=local/fixture", "--base-url=" + server.URL + "/v1", "--new-session", "-p", "perform requested skill operation"}
				if approve {
					flags = append(flags, "--yes")
				}
				cmd := exec.CommandContext(ctx, binary, flags...)
				cmd.Dir = workspace
				cmd.Env = append(os.Environ(), "HOME="+home)
				if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "fixture finished") {
					t.Fatal("binary failed", err, string(out))
				}
			}
			invoke(false)
			if calls.Load() != 2 {
				t.Fatal("unexpected request count", calls.Load())
			}
			<-requests
			denied := <-requests
			if !strings.Contains(denied, "scoped approval requires a decision") {
				t.Fatal("provider did not receive scoped denial", denied)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatal("unapproved skill changed", err)
			}
			if _, err := os.Stat(filepath.Join(root, "new-skill")); !os.IsNotExist(err) {
				t.Fatal("unapproved create wrote directory", err)
			}
			invoke(true)
			if calls.Load() != 4 {
				t.Fatal("unexpected approved request count", calls.Load())
			}
			if action == "remove" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("approved remove failed", err)
				}
			} else {
				if action == "create" {
					path = filepath.Join(root, "new-skill", "SKILL.md")
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) == string(before) {
					t.Fatal("approved mutation failed", err)
				}
			}
		})
	}
}
