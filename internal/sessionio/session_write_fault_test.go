//go:build darwin || linux

package sessionio

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/harness/session"
)

// Limits are changed only in a disposable child, never in the parent test or
// user's shell. EFBIG here comes from the kernel's real file-write syscall.
func TestSessionWriteLimitHelper(t *testing.T) {
	operation := os.Getenv("HAND_SESSION_WRITE_LIMIT_OPERATION")
	if operation == "" {
		return
	}
	root := os.Getenv("HAND_SESSION_WRITE_LIMIT_ROOT")
	m, err := NewManager(root, os.Getenv("HAND_SESSION_WRITE_LIMIT_WORKSPACE"), "hand")
	if err != nil {
		t.Fatal(err)
	}
	source, err := m.Create(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	source.Session.Append(session.UserMessageEntry(strings.Repeat("original content ", 8192)))
	if err := source.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	history := source.Session.History()
	source.Session.Close()
	sourcePath, err := m.SessionPath(source.Record)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	destination := filepath.Join(outDir, "result.jsonl")
	var original syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &original); err != nil {
		t.Fatal(err)
	}
	limit := original
	limit.Cur = 1024
	signal.Ignore(syscall.SIGXFSZ)
	defer signal.Reset(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_FSIZE, &original)
	switch operation {
	case "export":
		err = m.ExportID(context.Background(), source.Session.ID, destination)
	case "fork":
		_, err = m.Fork(context.Background(), history)
	default:
		t.Fatal("unknown operation")
	}
	if !errors.Is(err, syscall.EFBIG) {
		t.Fatalf("expected real file-size failure, got %v", err)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &original); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(outDir)
	if err != nil || len(files) != 0 {
		t.Fatal("failed export left output", files, err)
	}
	if _, err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.Catalogue().Snapshot()
	if err != nil || len(snapshot.Sessions) != 1 || snapshot.LastActiveID != source.Session.ID {
		t.Fatal("failed write published partial fork", snapshot, err)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed write changed source", err)
	}
	// Restoring the limit must permit a new export, including lease reacquisition.
	if err := m.ExportID(context.Background(), source.Session.ID, destination); err != nil {
		t.Fatal("retry after real write failure", err)
	}
	copied, err := os.ReadFile(destination)
	if err != nil || string(copied) != string(before) {
		t.Fatal("retry export corrupted", err)
	}
}

func TestSessionOperationsHandleRealWriteLimit(t *testing.T) {
	for _, operation := range []string{"export", "fork"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionWriteLimitHelper$")
			cmd.Env = append(os.Environ(), "HAND_SESSION_WRITE_LIMIT_OPERATION="+operation, "HAND_SESSION_WRITE_LIMIT_ROOT="+t.TempDir(), "HAND_SESSION_WRITE_LIMIT_WORKSPACE="+t.TempDir())
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("write-fault child: %v: %s", err, out)
			}
		})
	}
}
