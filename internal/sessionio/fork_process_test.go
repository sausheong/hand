package sessionio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sausheong/harness/session"
)

func TestForkProcessHelper(t *testing.T) {
	root := os.Getenv("HAND_FORK_PROCESS_ROOT")
	if root == "" {
		return
	}
	m, err := NewManager(root, os.Getenv("HAND_FORK_PROCESS_WORKSPACE"), "hand")
	if err != nil {
		t.Fatal(err)
	}
	m.forkCheckpoint = func(stage string) error {
		if stage == os.Getenv("HAND_FORK_PROCESS_STAGE") {
			fmt.Println("FORK_BOUNDARY")
			io.Copy(io.Discard, os.Stdin)
		}
		return nil
	}
	result, err := m.Fork(context.Background(), []session.SessionEntry{session.UserMessageEntry("first"), session.UserMessageEntry("second")})
	if err != nil {
		t.Fatal(err)
	}
	result.Session.Close()
}

func TestForkProcessDeathPublicationBoundaries(t *testing.T) {
	for _, stage := range []string{"copied_entry", "before_publish", "after_publish", "before_catalogue"} {
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
			source.Session.Append(session.UserMessageEntry("source unchanged"))
			if err := source.Session.Flush(); err != nil {
				t.Fatal(err)
			}
			path, err := m.SessionPath(source.Record)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Session.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestForkProcessHelper$")
			cmd.Env = append(os.Environ(), "HAND_FORK_PROCESS_ROOT="+root, "HAND_FORK_PROCESS_WORKSPACE="+workspace, "HAND_FORK_PROCESS_STAGE="+stage)
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
			if !scanner.Scan() || scanner.Text() != "FORK_BOUNDARY" {
				t.Fatal("child did not reach boundary")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("child was not killed")
			}
			for i := 0; i < 2; i++ {
				if _, err := m.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := m.Catalogue().Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			expected := 1
			if stage == "after_publish" || stage == "before_catalogue" {
				expected = 2
			}
			if len(snapshot.Sessions) != expected || snapshot.LastActiveID != source.Session.ID {
				t.Fatal("partial discovery, duplicate recovery or changed selection", snapshot)
			}
			for _, record := range snapshot.Sessions {
				if record.ID == source.Session.ID {
					continue
				}
				recovered, err := m.Open(context.Background(), record.ID, false)
				if err != nil {
					t.Fatal("dead worker lease retained", err)
				}
				if len(recovered.Session.History()) != 2 {
					t.Fatal("recovered partial fork")
				}
				recovered.Session.Append(session.UserMessageEntry("recovered continuation"))
				if err := recovered.Session.Flush(); err != nil {
					t.Fatal(err)
				}
				recovered.Session.Close()
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatal("source changed on fork interruption", err)
			}
		})
	}
}
