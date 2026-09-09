package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func TestLifecyclePeerProcess(t *testing.T) {
	mode := os.Getenv("HAND_LIFECYCLE_PEER")
	if mode == "" {
		return
	}
	reader, writer := protocol.NewReader(os.Stdin), protocol.NewWriter(os.Stdout)
	for {
		request, err := reader.Read()
		if err == io.EOF {
			os.Exit(0)
		}
		if err != nil {
			os.Exit(2)
		}
		reply := protocol.Frame{Version: 1, Kind: "response", ID: request.ID, Result: json.RawMessage(`{}`)}
		switch request.Method {
		case "initialize":
			reply.Result = json.RawMessage(`{"version":1,"name":"lifecycle-fixture","capabilities":["lifecycle"],"subscriptions":["run.start","run.finish"]}`)
			if strings.HasPrefix(mode, "sessions") {
				reply.Result = json.RawMessage(`{"version":1,"name":"lifecycle-fixture","capabilities":["lifecycle"],"subscriptions":["session.open","session.close"]}`)
			}
			if mode == "compaction" {
				reply.Result = json.RawMessage(`{"version":1,"name":"lifecycle-fixture","capabilities":["lifecycle"],"subscriptions":["context.compacted"]}`)
			}
		case "lifecycle.notify":
			var event struct {
				Event string                     `json:"event"`
				Data  map[string]json.RawMessage `json:"data"`
			}
			if err = protocol.DecodePayload(request.Params, &event); err != nil {
				os.Exit(3)
			}
			file, err := os.OpenFile(os.Getenv("HAND_LIFECYCLE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				os.Exit(4)
			}
			_, err = file.Write(append(append([]byte(nil), request.Params...), '\n'))
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				os.Exit(5)
			}
			if mode == "sessions-hang-close" && event.Event == "session.close" {
				time.Sleep(10 * time.Second)
			}
			if mode == "fail-start" && event.Event == "run.start" || mode == "fail-finish" && event.Event == "run.finish" {
				reply.Result = nil
				reply.Error = &protocol.Error{Code: "observer_failed", Message: "fixture observer rejected notification"}
			}
		default:
			os.Exit(6)
		}
		if err = writer.Write(reply); err != nil {
			os.Exit(7)
		}
	}
}
func TestExtensionLifecycleStartAdmissionAndFinishObservation(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, api := range []string{"run", "runturn"} {
		for _, mode := range []string{"allow", "fail-start", "fail-finish"} {
			t.Run(api+"-"+mode, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				sess := session.NewSession("hand", "lifecycle")
				defer sess.Close()
				provider := &extensionContextProvider{}
				rt := &runtime.Runtime{Session: sess, LLM: provider, Tools: tool.NewRegistry()}
				defer rt.Close()
				controller := &Controller{Rt: rt}
				workspace := t.TempDir()
				log := filepath.Join(t.TempDir(), "events.jsonl")
				review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "lifecycle-fixture", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestLifecyclePeerProcess$"}, Environment: map[string]string{"HAND_LIFECYCLE_PEER": mode, "HAND_LIFECYCLE_LOG": log}, Capabilities: []string{"lifecycle"}, Mandatory: true})
				if err != nil {
					t.Fatal(err)
				}
				host, err := controller.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "private"), Identities: map[string]string{"lifecycle-fixture": "fixture/lifecycle"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
				if err != nil {
					t.Fatal(err)
				}
				defer host.Close()
				failed := false
				if api == "run" {
					events, err := rt.Run(ctx, "hello", nil)
					if err != nil {
						t.Fatal(err)
					}
					for event := range events {
						if event.Error != nil {
							failed = true
						}
					}
				} else {
					result, err := rt.RunTurn(ctx, "hello", nil, nil)
					failed = err != nil || result.Err != nil
				}
				if failed != (mode == "fail-start") {
					t.Fatal("observer changed outcome", failed, mode)
				}
				expectedCalls := 1
				if mode == "fail-start" {
					expectedCalls = 0
				}
				if len(provider.requests) != expectedCalls {
					t.Fatal("provider admission", len(provider.requests))
				}
				raw, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
				if len(lines) != 2 {
					t.Fatalf("lifecycle count %d: %s", len(lines), raw)
				}
				for i, line := range lines {
					var event struct {
						Event string            `json:"event"`
						Data  map[string]string `json:"data"`
					}
					if err = json.Unmarshal([]byte(line), &event); err != nil {
						t.Fatal(err)
					}
					if event.Event != []string{"run.start", "run.finish"}[i] || event.Data["session_id"] != sess.ID {
						t.Fatal(event)
					}
					if i == 1 {
						reason := "completed"
						if mode == "fail-start" {
							reason = "error"
						}
						if event.Data["reason"] != reason {
							t.Fatal("finish reason", event)
						}
					}
				}
			})
		}
	}
}
