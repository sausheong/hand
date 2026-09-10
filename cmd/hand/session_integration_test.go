package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
)

func TestInvocationNewAndExplicitResumePreserveHistory(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		requests <- string(body["messages"])
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture answer\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	invocationFixture(t)
	invoke := func(extra ...string) {
		t.Helper()
		os.Args = append([]string{"hand", "--model=local/fixture", "--base-url=" + server.URL + "/v1"}, extra...)
		flag.CommandLine = flag.NewFlagSet("hand", flag.ContinueOnError)
		flag.CommandLine.SetOutput(io.Discard)
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	invoke("-p", "first conversation")
	<-requests
	root, err := sessionio.StoreDir()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := sessionio.OpenCatalogue(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := catalogue.Snapshot()
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatal("initial session not catalogued", err)
	}
	first := snapshot.LastActiveID
	invoke("--new-session", "-p", "second conversation")
	secondRequest := <-requests
	if strings.Contains(secondRequest, "first conversation") {
		t.Fatal("new session reused old context")
	}
	snapshot, err = catalogue.Snapshot()
	if err != nil || len(snapshot.Sessions) != 2 || snapshot.LastActiveID == first {
		t.Fatal("new deleted previous session", err)
	}
	invoke("--session="+first, "-p", "continue first")
	resumed := <-requests
	if !strings.Contains(resumed, "first conversation") || !strings.Contains(resumed, "continue first") || strings.Contains(resumed, "second conversation") {
		t.Fatal("explicit resume used wrong history", resumed)
	}
	snapshot, err = catalogue.Snapshot()
	if err != nil || snapshot.LastActiveID != first {
		t.Fatal("explicit selection not persisted", err)
	}
}
