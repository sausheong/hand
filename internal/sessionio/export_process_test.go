package sessionio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/harness/session"
)

func TestExportProcessHelper(t *testing.T) {
	root := os.Getenv("HAND_EXPORT_PROCESS_ROOT")
	if root == "" {
		return
	}
	m, err := NewManager(root, os.Getenv("HAND_EXPORT_PROCESS_WORKSPACE"), "hand")
	if err != nil {
		t.Fatal(err)
	}
	m.exportCheckpoint = func(stage string) error {
		if stage == os.Getenv("HAND_EXPORT_PROCESS_STAGE") {
			fmt.Println("EXPORT_BOUNDARY")
			io.Copy(io.Discard, os.Stdin)
		}
		return nil
	}
	if err := m.ExportID(context.Background(), os.Getenv("HAND_EXPORT_PROCESS_ID"), os.Getenv("HAND_EXPORT_PROCESS_DEST")); err != nil {
		t.Fatal(err)
	}
}

func TestExportProcessDeathPublicationBoundaries(t *testing.T) {
	for _, stage := range []string{"before_publish", "after_publish"} {
		t.Run(stage, func(t *testing.T) {
			root, workspace := t.TempDir(), t.TempDir()
			m, err := NewManager(root, workspace, "hand")
			if err != nil {
				t.Fatal(err)
			}
			source, err := m.Create(context.Background(), "source")
			if err != nil {
				t.Fatal(err)
			}
			source.Session.Append(session.UserMessageEntry("export source"))
			if err := source.Session.Flush(); err != nil {
				t.Fatal(err)
			}
			source.Session.Close()
			path, err := m.SessionPath(source.Record)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(t.TempDir(), "result.jsonl")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExportProcessHelper$")
			cmd.Env = append(os.Environ(), "HAND_EXPORT_PROCESS_ROOT="+root, "HAND_EXPORT_PROCESS_WORKSPACE="+workspace, "HAND_EXPORT_PROCESS_STAGE="+stage, "HAND_EXPORT_PROCESS_ID="+source.Session.ID, "HAND_EXPORT_PROCESS_DEST="+destination)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() || scanner.Text() != "EXPORT_BOUNDARY" {
				t.Fatal("child failed to reach boundary")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("child was not killed")
			}
			exported, err := os.ReadFile(destination)
			if stage == "before_publish" {
				if !os.IsNotExist(err) {
					t.Fatal("destination exposed before publication", err)
				}
			} else if err != nil || string(exported) != string(before) {
				t.Fatal("published export was incomplete", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatal("export interruption mutated source", err)
			}
			retry := filepath.Join(t.TempDir(), "retry.jsonl")
			if err := m.ExportID(context.Background(), source.Session.ID, retry); err != nil {
				t.Fatal("dead exporter retained lease", err)
			}
			copied, err := os.ReadFile(retry)
			if err != nil || string(copied) != string(before) {
				t.Fatal("retry corrupted export", err)
			}
		})
	}
}
