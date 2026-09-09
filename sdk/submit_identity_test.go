package sdk

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/protocol"
	"net"
	"testing"
	"time"
)

func TestSubmitBindsExecutionToRequest(t *testing.T) {
	for _, id := range []string{"sent", "other"} {
		for _, state := range []string{"pending", "accepted", "uncertain"} {
			t.Run(id+"-"+state, func(t *testing.T) {
				server, peer := net.Pipe()
				defer server.Close()
				c := NewClient(peer)
				defer c.Close()
				done := make(chan error, 1)
				go func() {
					raw, err := protocol.NewReader(server).ReadFrame()
					if err != nil {
						done <- err
						return
					}
					req, err := protocol.DecodeRequest(raw)
					if err != nil {
						done <- err
						return
					}
					result, _ := json.Marshal(map[string]string{"id": id, "session_id": "s", "run_id": "r", "state": state})
					done <- protocol.NewWriter(server).Write(protocol.Response{Version: 1, RequestID: req.ID, Result: result})
				}()
				request, err := NewPromptRequest("sent", "do this task")
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				execution, err := c.Submit(ctx, request)
				if id == "other" {
					if err == nil {
						t.Fatal("accepted another execution", execution)
					}
				} else if err != nil || execution.ID() != id || execution.State() != state {
					t.Fatal("correct execution rejected", execution, err)
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
