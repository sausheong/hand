//go:build unix

package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/process"
)

func TestProcessToolLifecycle(t *testing.T) {
	p := testProcesses(t)
	tool := &ProcessTool{Processes: p}
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"start","command":"read line; printf '%s' \"$line\""}`))
	if err != nil {
		t.Fatal(err)
	}
	var info ProcessInfo
	if err = json.Unmarshal([]byte(out.Output), &info); err != nil || info.ID == "" {
		t.Fatalf("handle: %s %v", out.Output, err)
	}
	call := func(action string, extra map[string]any) string {
		t.Helper()
		in := map[string]any{"action": action, "id": info.ID}
		for k, v := range extra {
			in[k] = v
		}
		b, _ := json.Marshal(in)
		result, err := tool.Execute(context.Background(), b)
		if err != nil {
			t.Fatal(err)
		}
		return result.Output
	}
	var s process.HandleSnapshot
	if err = json.Unmarshal([]byte(call("wait", map[string]any{"wait_ms": 1})), &s); err != nil || !s.Running {
		t.Fatalf("bounded wait: %+v %v", s, err)
	}
	call("send", map[string]any{"data": "hello\n"})
	if err = json.Unmarshal([]byte(call("wait", map[string]any{"wait_ms": 5000})), &s); err != nil || s.Running || s.Stdout != "hello" {
		t.Fatalf("completion: %+v %v", s, err)
	}
	call("forget", nil)
}
func TestProcessToolPermissionClassification(t *testing.T) {
	hook := agentio.NewOneShotApprovalHook(nil, false, nil, nil)
	for _, action := range []string{"start", "send", "cancel", "forget", "unknown", ""} {
		b, _ := json.Marshal(map[string]string{"action": action})
		d, err := hook(context.Background(), "process", b)
		if err != nil || d.Allow {
			t.Fatalf("%s bypassed approval: %+v %v", action, d, err)
		}
	}
	for _, action := range []string{"list", "read", "wait"} {
		b, _ := json.Marshal(map[string]string{"action": action})
		d, err := hook(context.Background(), "process", b)
		if err != nil || !d.Allow {
			t.Fatalf("%s: %+v %v", action, d, err)
		}
	}
}
func TestProcessToolRejectsInvalidInputBeforeStart(t *testing.T) {
	p := testProcesses(t)
	tool := &ProcessTool{Processes: p}
	for _, input := range []string{`{"action":"start","command":"true","extra":1}`, `{"action":"start","command":"true"} {}`, `{"action":"wait","wait_ms":30001}`, `{"action":"unknown"}`} {
		if _, err := tool.Execute(context.Background(), json.RawMessage(input)); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	if len(p.List()) != 0 {
		t.Fatal("invalid request started process")
	}
}
