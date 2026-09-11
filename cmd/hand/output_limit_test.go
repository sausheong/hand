package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOutputLimitFlagReachesProvider(t *testing.T) {
	limits := make(chan int, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Limit int `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		limits <- body.Limit
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"Answer"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	invocationFixture(t)
	os.Args = []string{"hand", "--model=local/fixture", "--base-url=" + server.URL + "/v1", "--context-limit=1000000", "--max-output=8192", "-p", "question"}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-limits:
		if got != 8192 {
			t.Fatalf("output allowance %d", got)
		}
	default:
		t.Fatal("no request")
	}
	if len(limits) != 0 {
		t.Fatal("unexpected extra request")
	}
}
