package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
)

func TestBinaryNewSessionAndResumeAcrossProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v: %s", err, out)
	}
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		requests <- string(body["messages"])
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"binary answer\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	home, workspace := t.TempDir(), t.TempDir()
	invoke := func(args ...string) {
		t.Helper()
		flags := append([]string{"--model=local/fixture", "--base-url=" + server.URL + "/v1"}, args...)
		cmd := exec.CommandContext(ctx, binary, flags...)
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("binary invocation: %v: %s", err, out)
		}
		if !strings.Contains(string(out), "binary answer") {
			t.Fatalf("missing answer: %s", out)
		}
	}
	invoke("-p", "binary original conversation")
	<-requests
	catalogue, err := sessionio.OpenCatalogue(filepath.Join(home, ".hand", "sessions"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	first, err := catalogue.Snapshot()
	if err != nil || len(first.Sessions) != 1 {
		t.Fatal("first process did not save session", err)
	}
	id := first.LastActiveID
	invoke("--new-session", "-p", "binary alternate conversation")
	alternate := <-requests
	if strings.Contains(alternate, "binary original conversation") {
		t.Fatal("new process reused original context")
	}
	second, err := catalogue.Snapshot()
	if err != nil || len(second.Sessions) != 2 || second.LastActiveID == id {
		t.Fatal("new process replaced original session", err)
	}
	invoke("--session="+id, "-p", "binary resume original")
	resumed := <-requests
	if !strings.Contains(resumed, "binary original conversation") || strings.Contains(resumed, "binary alternate conversation") {
		t.Fatal("resumed process loaded wrong context")
	}
	final, err := catalogue.Snapshot()
	if err != nil || final.LastActiveID != id || len(final.Sessions) != 2 {
		t.Fatal("resume did not retain both sessions", err)
	}
}
