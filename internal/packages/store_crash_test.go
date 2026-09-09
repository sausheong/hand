package packages

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type pauseBeforePackageRename struct {
	context.Context
	directory string
}

func (c *pauseBeforePackageRename) Err() error {
	entries, _ := os.ReadDir(c.directory)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "lock-") && strings.HasSuffix(entry.Name(), ".tmp") {
			fmt.Println("HAND_PACKAGE_CRASH_READY")
			time.Sleep(time.Hour)
		}
	}
	return c.Context.Err()
}
func TestPackageStoreCrashPeer(t *testing.T) {
	mode := os.Getenv("HAND_PACKAGE_CRASH_MODE")
	if mode == "" {
		return
	}
	directory := os.Getenv("HAND_PACKAGE_CRASH_STORE")
	store, err := OpenStore(directory, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	review, err := store.PrepareRemove("task-note")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := review.ApprovalDigest()
	if err != nil {
		t.Fatal(err)
	}
	var ctx context.Context = context.Background()
	if mode == "before-rename" {
		ctx = &pauseBeforePackageRename{Context: ctx, directory: directory}
	}
	result, err := store.Apply(ctx, review, approval)
	if err != nil || !result.Committed {
		t.Fatal(result, err)
	}
	fmt.Println("HAND_PACKAGE_CRASH_READY")
	time.Sleep(time.Hour)
}
func TestPackageStoreProcessDeathPreservesCommitBoundary(t *testing.T) {
	for _, mode := range []string{"before-rename", "after-commit"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateStageParent(t)
			source := writeFixture(t)
			store, err := OpenStore(directory, "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			_, pin, err := VerifyDirectory(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			install, err := store.PrepareInstall(context.Background(), source, pin)
			if err != nil {
				t.Fatal(err)
			}
			approveChange(t, store, install)
			before, err := os.ReadFile(filepath.Join(directory, "lock.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			child := exec.Command(executable, "-test.run=^TestPackageStoreCrashPeer$", "-test.timeout=20s")
			child.Env = append(os.Environ(), "HAND_PACKAGE_CRASH_MODE="+mode, "HAND_PACKAGE_CRASH_STORE="+directory)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					child.Process.Kill()
					child.Wait()
				}
			}()
			ready := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if scanner.Text() == "HAND_PACKAGE_CRASH_READY" {
						ready <- nil
						return
					}
				}
				ready <- fmt.Errorf("crash peer ended before readiness: %v", scanner.Err())
			}()
			select {
			case err = <-ready:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("crash peer did not reach commit boundary")
			}
			if other, err := OpenStore(directory, "1.0.0"); !errors.Is(err, ErrStoreBusy) || other != nil {
				if other != nil {
					other.Close()
				}
				t.Fatal("live child did not own writer lease", err)
			}
			if err = child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = child.Wait()
			waited = true
			if err == nil {
				t.Fatal("crash peer was not killed")
			}
			reopened, err := OpenStore(directory, "1.0.0")
			if err != nil {
				t.Fatal("dead writer lease not released", err)
			}
			defer reopened.Close()
			state, err := reopened.List()
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(directory, "lock.json"))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "before-rename" {
				if state.Generation != 1 || len(state.Packages) != 1 || state.Packages[0].Current != pin || string(before) != string(after) {
					t.Fatal("uncommitted replacement became authoritative", state)
				}
				files, err := os.ReadDir(directory)
				if err != nil {
					t.Fatal(err)
				}
				temporary := false
				for _, file := range files {
					if strings.HasPrefix(file.Name(), "lock-") && strings.HasSuffix(file.Name(), ".tmp") {
						temporary = true
					}
				}
				if temporary || reopened.Recovery().RemovedTemporaryLocks != 1 {
					t.Fatal("stale pre-rename lock not recovered", reopened.Recovery())
				}
				remove, err := reopened.PrepareRemove(install.Name)
				if err != nil {
					t.Fatal(err)
				}
				approveChange(t, reopened, remove)
			} else if state.Generation != 2 || len(state.Packages) != 0 {
				t.Fatal("committed removal lost after process death", state)
			}
			if _, got, err := VerifyDirectory(context.Background(), filepath.Join(directory, "objects", pin)); err != nil || got != pin {
				t.Fatal("retained content damaged by writer death", got, err)
			}
		})
	}
}
