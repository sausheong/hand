package sessionio

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalogueLockHelper(t *testing.T) {
	path := os.Getenv("HAND_CATALOGUE_LOCK_HELPER")
	if path == "" {
		return
	}
	lock, err := lockCatalogue(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Println("CATALOGUE_LOCKED")
	io.Copy(io.Discard, os.Stdin)
}
func TestCatalogueLockReleasedOnProcessDeath(t *testing.T) {
	c := catalogueFixture(t)
	if err := c.Register(catalogueRecord("one"), true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCatalogueLockHelper$")
	cmd.Env = append(os.Environ(), "HAND_CATALOGUE_LOCK_HELPER="+filepath.Join(c.dir, "catalogue.lock"))
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
	if !scanner.Scan() || scanner.Text() != "CATALOGUE_LOCKED" {
		t.Fatal("child did not lock catalogue")
	}
	if err := c.Select("one"); !errors.Is(err, ErrCatalogueBusy) {
		t.Fatal("live child lock bypassed", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if err := c.Register(catalogueRecord("two"), true); err != nil {
		t.Fatal("dead process left catalogue locked", err)
	}
	s, err := c.Snapshot()
	if err != nil || len(s.Sessions) != 2 || s.LastActiveID != "two" {
		t.Fatal("restart lost catalogue state", err)
	}
}

func TestCatalogueCommitHelper(t *testing.T) {
	dir := os.Getenv("HAND_CATALOGUE_COMMIT_HELPER_DIR")
	if dir == "" {
		return
	}
	c := &Catalogue{dir: dir, workspace: os.Getenv("HAND_CATALOGUE_COMMIT_HELPER_WORKSPACE")}
	stage := os.Getenv("HAND_CATALOGUE_COMMIT_HELPER_STAGE")
	ops := c.operations()
	rename := ops.rename
	ops.rename = func(from, to string) error {
		if stage == "after" {
			if err := rename(from, to); err != nil {
				return err
			}
		}
		fmt.Println("CATALOGUE_COMMIT_READY")
		io.Copy(io.Discard, os.Stdin)
		if stage == "before" {
			return rename(from, to)
		}
		return nil
	}
	c.io = &ops
	if err := c.Register(catalogueRecord("two"), false); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogueCommitSurvivesProcessDeath(t *testing.T) {
	for _, stage := range []string{"before", "after"} {
		t.Run(stage, func(t *testing.T) {
			c := catalogueFixture(t)
			if err := c.Register(catalogueRecord("one"), false); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCatalogueCommitHelper$")
			cmd.Env = append(os.Environ(), "HAND_CATALOGUE_COMMIT_HELPER_DIR="+c.dir, "HAND_CATALOGUE_COMMIT_HELPER_WORKSPACE="+c.workspace, "HAND_CATALOGUE_COMMIT_HELPER_STAGE="+stage)
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
			if !scanner.Scan() || scanner.Text() != "CATALOGUE_COMMIT_READY" {
				t.Fatal("child did not reach commit boundary")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			cmd.Wait()
			snapshot, err := c.Snapshot()
			if err != nil {
				t.Fatal("interruption left corrupt catalogue", err)
			}
			expected := 1
			if stage == "after" {
				expected = 2
			}
			if len(snapshot.Sessions) != expected || snapshot.Sessions[0].ID != "one" {
				t.Fatal("wrong atomic commit boundary", snapshot)
			}
			if err := c.Register(catalogueRecord("two"), false); err != nil {
				t.Fatal("restart failed", err)
			}
			snapshot, err = c.Snapshot()
			if err != nil || len(snapshot.Sessions) != 2 || snapshot.Generation != 2 {
				t.Fatal("restart duplicated/lost registration", snapshot, err)
			}
		})
	}
}
