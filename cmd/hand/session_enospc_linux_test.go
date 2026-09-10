//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/session"
)

// The acceptance runner supplies a disposable, size-limited tmpfs. Never fill
// an arbitrary host filesystem or change limits on the parent process.
func TestBinaryExportReportsRealENOSPC(t *testing.T) {
	root, binary := os.Getenv("HAND_TEST_ENOSPC_ROOT"), os.Getenv("HAND_TEST_ENOSPC_BINARY")
	if root == "" || binary == "" {
		t.Skip("explicit disposable tmpfs and built Hand binary required")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Type != 0x01021994 || fs.Blocks*uint64(fs.Bsize) > 8<<20 {
		t.Fatal("test requires tmpfs of at most 8 MiB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	home := filepath.Join(root, "home")
	workspace := t.TempDir()
	manager, err := sessionio.NewManager(filepath.Join(home, ".hand", "sessions"), workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	source, err := manager.Create(ctx, "source")
	if err != nil {
		t.Fatal(err)
	}
	source.Session.Append(session.UserMessageEntry(strings.Repeat("retained history ", 4096)))
	if err := source.Session.Close(); err != nil {
		t.Fatal(err)
	}
	active, err := manager.Create(ctx, "active")
	if err != nil {
		t.Fatal(err)
	}
	if err := active.Session.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := manager.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	sourcePath, err := manager.SessionPath(source.Record)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".hand", "config.json"), []byte("invalid provider configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	fillerPath := filepath.Join(root, "filler")
	filler, err := os.OpenFile(fillerPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fillerPath)
	block := make([]byte, 64<<10)
	filled := false
	for written := 0; written < 16<<20; written += len(block) {
		if _, err = filler.Write(block); err != nil {
			if !errors.Is(err, syscall.ENOSPC) {
				t.Fatal(err)
			}
			filled = true
			break
		}
	}
	if err := filler.Close(); err != nil {
		t.Fatal(err)
	}
	if !filled {
		t.Fatal("bounded filesystem did not report ENOSPC")
	}
	destination := filepath.Join(root, "export.jsonl")
	invoke := func() ([]byte, error) {
		cmd := exec.CommandContext(ctx, binary, "--session", source.Session.ID, "--export-session", destination)
		cmd.Dir = workspace
		cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
		return cmd.CombinedOutput()
	}
	output, err := invoke()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 5 || !strings.Contains(string(output), "no space left on device") {
		t.Fatalf("missing visible disk-full error: %v: %s", err, output)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial export published: %v", err)
	}
	after, err := manager.Catalogue().Snapshot()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("full disk changed catalogue: %v", err)
	}
	retained, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.Equal(original, retained) {
		t.Fatalf("full disk changed source: %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".hand-export-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary export leaked: %v %v", leftovers, err)
	}
	if err := os.Remove(fillerPath); err != nil {
		t.Fatal(err)
	}
	if output, err := invoke(); err != nil {
		t.Fatalf("export retry after freeing space: %v: %s", err, output)
	}
	exported, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(original, exported) {
		t.Fatalf("retry export changed data: %v", err)
	}
	t.Log("Kernel ENOSPC reached; CLI exit 5 and disk-full diagnostic; no partial publication; source/catalogue preserved; retry byte-identical")
}

func TestBinaryConversationReportsRealENOSPC(t *testing.T) {
	root, binary := os.Getenv("HAND_TEST_ENOSPC_ROOT"), os.Getenv("HAND_TEST_ENOSPC_BINARY")
	if root == "" || binary == "" {
		t.Skip("explicit disposable tmpfs and built Hand binary required")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Type != 0x01021994 || fs.Blocks*uint64(fs.Bsize) > 8<<20 {
		t.Fatal("test requires tmpfs of at most 8 MiB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	home := filepath.Join(root, "conversation-home")
	workspace := t.TempDir()
	manager, err := sessionio.NewManager(filepath.Join(home, ".hand", "sessions"), workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Create(ctx, "retained")
	if err != nil {
		t.Fatal(err)
	}
	selected.Session.Append(session.UserMessageEntry("original durable conversation"))
	if err := selected.Session.Close(); err != nil {
		t.Fatal(err)
	}
	sourcePath, err := manager.SessionPath(selected.Record)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	fillerPath := filepath.Join(root, "conversation-filler")
	defer os.Remove(fillerPath)
	filled := make(chan error, 1)
	reportFill := func(err error) {
		select {
		case filled <- err:
		default:
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.OpenFile(fillerPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			reportFill(err)
			http.Error(w, "fixture setup", 500)
			return
		}
		block := make([]byte, 64<<10)
		for written := 0; written < 16<<20; written += len(block) {
			if _, err = f.Write(block); err != nil {
				break
			}
		}
		closeErr := f.Close()
		if !errors.Is(err, syscall.ENOSPC) || closeErr != nil {
			reportFill(fmt.Errorf("expected kernel ENOSPC, got %v; close %v", err, closeErr))
			http.Error(w, "fixture exhaustion", 500)
			return
		}
		reportFill(nil)
		w.Header().Set("Content-Type", "text/event-stream")
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": strings.Repeat("unsaved answer ", 4096)}, "finish_reason": nil}}})
		fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()
	cmd := exec.CommandContext(ctx, binary, "--model=local/fixture", "--base-url="+server.URL+"/v1", "--session="+selected.Session.ID, "-p", "active conversation question")
	cmd.Dir = workspace
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	output, err := cmd.CombinedOutput()
	select {
	case fillErr := <-filled:
		if fillErr != nil {
			t.Fatal(fillErr)
		}
	default:
		t.Fatalf("provider did not receive active request: %v: %s", err, output)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 5 || !strings.Contains(string(output), "no space left on device") {
		t.Fatalf("missing conversation persistence failure: %v: %s", err, output)
	}
	retained, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.HasPrefix(retained, original) {
		t.Fatalf("previous durable journal changed: %v", err)
	}
	if err := os.Remove(fillerPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Open(ctx, selected.Session.ID, false)
	if err != nil {
		t.Fatal("recover after disk full", err)
	}
	defer reopened.Session.Close()
	history := reopened.Session.History()
	joined := ""
	for _, entry := range history {
		joined += string(entry.Data)
	}
	if !strings.Contains(joined, "original durable conversation") || !strings.Contains(joined, "active conversation question") {
		t.Fatal("restart lost durable questions", joined)
	}
	reopened.Session.Append(session.UserMessageEntry("after space restored"))
	if err := reopened.Session.Flush(); err != nil {
		t.Fatal("recovered writer unusable", err)
	}
	t.Log("Provider request reached before kernel ENOSPC; CLI exit 5 and disk-full diagnostic; durable questions recovered and writer usable")
}
