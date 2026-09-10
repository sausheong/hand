package checkpoints

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

func TestStoreKilledBeforePublication(t *testing.T) {
	if os.Getenv("HAND_CHECKPOINT_CRASH_CHILD") == "1" {
		store, err := OpenStore(os.Getenv("HAND_CHECKPOINT_CRASH_STORE"), os.Getenv("HAND_CHECKPOINT_CRASH_WORK"), DefaultStoreLimits())
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		snap, err := Capture(context.Background(), os.Getenv("HAND_CHECKPOINT_CRASH_WORK"), DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		store.publish = func(old, new string) error {
			// Save has written, synced and closed the real temporary snapshot here.
			fmt.Println("CHECKPOINT_READY_TO_PUBLISH")
			time.Sleep(30 * time.Second)
			return errors.New("parent did not kill crash fixture")
		}
		if os.Getenv("HAND_CHECKPOINT_CRASH_PHASE") == "after" {
			store.publish = store.root.Rename
			store.syncDir = func() error {
				fmt.Println("CHECKPOINT_READY_TO_PUBLISH")
				time.Sleep(30 * time.Second)
				return errors.New("parent did not kill published fixture")
			}
		}
		if err = store.Save(context.Background(), snap); err != nil {
			t.Fatal(err)
		}
		return
	}
	work := t.TempDir()
	put(t, work, "file", "accepted before crash", 0600)
	before := capture(t, work)
	store, dir := newStore(t, work, DefaultStoreLimits())
	if err := store.Save(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	store.Close()
	put(t, work, "file", "unpublished after-image", 0600)
	after := capture(t, work)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreKilledBeforePublication$", "-test.count=1")
	cmd.Env = append(os.Environ(), "HAND_CHECKPOINT_CRASH_CHILD=1", "HAND_CHECKPOINT_CRASH_STORE="+dir, "HAND_CHECKPOINT_CRASH_WORK="+work)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		for scan.Scan() {
			if scan.Text() == "CHECKPOINT_READY_TO_PUBLISH" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			cmd.Wait()
			t.Fatalf("child stopped before publication barrier: %s", stderr.String())
		}
	case <-ctx.Done():
		cmd.Wait()
		t.Fatal("publication barrier timed out")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil {
		t.Fatal("crash child unexpectedly succeeded")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".pending-*"))
	if err != nil || len(matches) != 1 {
		t.Fatal("actual unpublished write missing", matches, err)
	}
	recovered, err := OpenStore(dir, work, DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if _, err = recovered.Load(context.Background(), before.Digest()); err != nil {
		t.Fatal("accepted snapshot lost", err)
	}
	if _, err = recovered.Load(context.Background(), after.Digest()); !os.IsNotExist(err) {
		t.Fatal("unpublished snapshot appeared committed", err)
	}
	matches, _ = filepath.Glob(filepath.Join(dir, ".pending-*"))
	if len(matches) != 0 {
		t.Fatal("orphan not recovered", matches)
	}
	if err = recovered.Save(context.Background(), after); err != nil {
		t.Fatal("recovered store unusable", err)
	}
	actual, _ := os.ReadFile(filepath.Join(work, "file"))
	if string(actual) != "unpublished after-image" {
		t.Fatal("recovery mutated workspace")
	}
}

func TestStorePublicationFailurePreservesAcceptedState(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "accepted", 0600)
	before := capture(t, work)
	store, dir := newStore(t, work, DefaultStoreLimits())
	if err := store.Save(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	put(t, work, "file", "new", 0600)
	after := capture(t, work)
	injected := errors.New("injected publication failure")
	store.publish = func(string, string) error { return injected }
	if err := store.Save(context.Background(), after); !errors.Is(err, injected) {
		t.Fatal("publication error lost", err)
	}
	if _, err := store.Load(context.Background(), before.Digest()); err != nil {
		t.Fatal(err)
	}
	ids, err := store.List()
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".pending-*"))
	if len(matches) != 0 {
		t.Fatal("failed publication leaked pending data")
	}
}

func TestStoreKilledAfterPublication(t *testing.T) {
	work := t.TempDir()
	put(t, work, "file", "published", 0600)
	snap := capture(t, work)
	store, dir := newStore(t, work, DefaultStoreLimits())
	store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreKilledBeforePublication$", "-test.count=1")
	cmd.Env = append(os.Environ(), "HAND_CHECKPOINT_CRASH_CHILD=1", "HAND_CHECKPOINT_CRASH_PHASE=after", "HAND_CHECKPOINT_CRASH_STORE="+dir, "HAND_CHECKPOINT_CRASH_WORK="+work)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		for scan.Scan() {
			if scan.Text() == "CHECKPOINT_READY_TO_PUBLISH" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			cmd.Wait()
			t.Fatal("child did not reach after-publication barrier")
		}
	case <-ctx.Done():
		cmd.Wait()
		t.Fatal("publication barrier timed out")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil {
		t.Fatal("child unexpectedly succeeded")
	}
	reopened, err := OpenStore(dir, work, DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.Load(context.Background(), snap.Digest())
	if err != nil {
		t.Fatal("published checkpoint not recovered", err)
	}
	if loaded.Digest() != snap.Digest() {
		t.Fatal("recovered wrong checkpoint")
	}
	pending, _ := filepath.Glob(filepath.Join(dir, ".pending-*"))
	if len(pending) != 0 {
		t.Fatal("published snapshot left pending entry")
	}
}
