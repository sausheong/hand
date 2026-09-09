package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/extension/protocol"
)

func TestVerificationHookChecksStagedAndUnstagedWhitespace(t *testing.T) {
	for _, mode := range []string{"clean", "unstaged", "staged", "cancelled", "not-repository"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
			}
			if mode != "not-repository" {
				git("init", "-q")
				if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("clean\n"), 0600); err != nil {
					t.Fatal(err)
				}
				git("add", "file.txt")
			}
			if mode == "staged" || mode == "unstaged" {
				if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("trailing whitespace \n"), 0600); err != nil {
					t.Fatal(err)
				}
				if mode == "staged" {
					git("add", "file.txt")
				}
			}
			ctx := context.Background()
			if mode == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := check(ctx, dir)
			if (err == nil) != (mode == "clean") {
				t.Fatal(mode, err)
			}
		})
	}
}
func TestVerificationHookProtocolContract(t *testing.T) {
	value, err := handle(protocol.Frame{Method: "initialize"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hello := value.(protocol.Hello)
	if err = hello.Validate([]string{"commands", "lifecycle"}); err != nil {
		t.Fatal(err)
	}
	if _, err = handle(protocol.Frame{Method: "command.execute", Params: []byte(`{"name":"verify-whitespace","arguments":"arbitrary command"}`)}, t.TempDir()); err == nil {
		t.Fatal("arguments accepted")
	}
	if _, err = handle(protocol.Frame{Method: "lifecycle.notify", Params: []byte(`{"event":"run.finish","data":{}}`)}, t.TempDir()); err == nil {
		t.Fatal("nonrepository verification reported success")
	}
}
