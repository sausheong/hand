package toolworker

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerRealFileRoundtripAndRejectedRequests(t *testing.T) {
	workspace := t.TempDir()
	call := func(name string, input any) Response {
		t.Helper()
		data, _ := json.Marshal(input)
		raw, _ := json.Marshal(Request{Version: 1, Tool: name, Input: data})
		var out bytes.Buffer
		if err := Serve(context.Background(), workspace, bytes.NewReader(raw), &out); err != nil {
			t.Fatal(err)
		}
		var response Response
		if err := json.Unmarshal(out.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Version != 1 || response.Result.Error != "" {
			t.Fatalf("%+v", response)
		}
		return response
	}
	call("write_file", map[string]any{"path": "file.txt", "content": "before"})
	response := call("read_file", map[string]any{"path": "file.txt"})
	if !strings.Contains(response.Result.Output, "before") {
		t.Fatal(response)
	}
	call("edit_file", map[string]any{"path": "file.txt", "old_string": "before", "new_string": "after"})
	raw, err := os.ReadFile(filepath.Join(workspace, "file.txt"))
	if err != nil || string(raw) != "after" {
		t.Fatalf("%q %v", raw, err)
	}
	for _, request := range []string{
		`{"version":2,"tool":"write_file","input":{"path":"denied","content":"x"}}`,
		`{"version":1,"tool":"bash","input":{"command":"touch denied"}}`,
		`{"version":1,"tool":"write_file","input":{"path":"denied","content":"x"},"extra":true}`,
		`{"version":1,"tool":"write_file","input":null}`,
		`{"version":1,"tool":"write_file","input":{"path":"denied","content":"x"}} {}`,
		strings.Repeat(" ", MaxRequestBytes+1),
	} {
		var out bytes.Buffer
		if err := Serve(context.Background(), workspace, strings.NewReader(request), &out); err == nil {
			t.Fatal("invalid request accepted")
		}
		if out.Len() != 0 {
			t.Fatal("invalid request emitted success")
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "denied")); !os.IsNotExist(err) {
		t.Fatal("rejected request mutated workspace")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestWorkerCancellationAndBrokenOutput(t *testing.T) {
	workspace := t.TempDir()
	raw := `{"version":1,"tool":"write_file","input":{"path":"file","content":"x"}}`
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Serve(ctx, workspace, strings.NewReader(raw), io.Discard); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "file")); !os.IsNotExist(err) {
		t.Fatal("cancelled request mutated")
	}
	if err := Serve(context.Background(), workspace, strings.NewReader(raw), shortWriter{}); err != io.ErrShortWrite {
		t.Fatal(err)
	}
}

func TestWorkerPreservesImageBytes(t *testing.T) {
	workspace := t.TempDir()
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "fixture.png"), imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Serve(context.Background(), workspace, strings.NewReader(`{"version":1,"tool":"read_file","input":{"path":"fixture.png"}}`), &out); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Images) != 1 || response.Images[0].MimeType != "image/png" || !bytes.Equal(response.Images[0].Data, imageBytes.Bytes()) {
		t.Fatalf("image bytes lost: %+v", response)
	}
}

func TestWorkerDoesNotSubstituteUnapprovedPathSpelling(t *testing.T) {
	workspace := t.TempDir()
	actual := filepath.Join(workspace, "narrow\u00a0space")
	if err := os.WriteFile(actual, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"read_file", "edit_file"} {
		raw, _ := json.Marshal(Request{Version: 1, Tool: name, Input: json.RawMessage(`{"path":"narrow space","old_string":"original","new_string":"changed"}`)})
		var out bytes.Buffer
		if err := Serve(context.Background(), workspace, bytes.NewReader(raw), &out); err != nil {
			t.Fatal(err)
		}
		var response Response
		json.Unmarshal(out.Bytes(), &response)
		if response.Result.Error == "" {
			t.Fatal("different path spelling silently accepted")
		}
	}
	raw, err := os.ReadFile(actual)
	if err != nil || string(raw) != "original" {
		t.Fatal("unapproved path changed")
	}
}
