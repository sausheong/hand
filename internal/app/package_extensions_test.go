package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/packages"
)

func TestPackageLaunchReviewRetainsExecutableResourcesWithoutExecuting(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "executed")
	content := []byte("#!/bin/sh\ntouch '" + sentinel + "'\n")
	sum := sha256.Sum256(content)
	m := packages.Manifest{Schema: 1, Name: "review-test", Version: "1.0.0", Compatibility: packages.Compatibility{MinimumHand: "1.0.0", ExtensionProtocol: 1}, Extensions: []packages.Extension{{Name: "review-test", Entrypoint: "main", Arguments: []string{"${package}/helper"}, Capabilities: []string{"commands"}}}}
	for _, name := range []string{"main", "helper"} {
		if err := os.WriteFile(filepath.Join(source, name), content, 0700); err != nil {
			t.Fatal(err)
		}
		m.Files = append(m.Files, packages.File{Path: name, Kind: "extension", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)), Executable: true})
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, packages.ManifestName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, pin, err := packages.VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "store")
	store, err := packages.OpenStore(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	change, err := store.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := change.ApprovalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, change, approval); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	snapshots := filepath.Join(t.TempDir(), "snapshots")
	review, err := ReviewPackageExtensions(ctx, store, []string{m.Name}, workspace, snapshots, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Reviews) != 1 || review.Identities[m.Name] != "package:review-test:review-test" {
		t.Fatal(review)
	}
	r := review.Reviews[0]
	if len(r.Assets) != 1 || r.Assets[0].Relative != "helper" || r.Assets[0].Mode&0111 == 0 || r.Binary.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal(r)
	}
	if _, err = os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatal("review executed package", err)
	}
	if _, err = os.Stat(snapshots); !os.IsNotExist(err) {
		t.Fatal("review activated snapshots", err)
	}
	if _, err = ReviewPackageExtensions(ctx, store, []string{m.Name}, workspace, snapshots, []PackageRuntimeApproval{{Package: m.Name}}); err == nil {
		t.Fatal("undeclared runtime approval accepted")
	}
	if _, err = ReviewPackageExtensions(ctx, store, []string{m.Name, m.Name}, workspace, snapshots, nil); err == nil {
		t.Fatal("duplicate selection accepted")
	}
}

// This journey builds the shipped extension, installs its pinned package, removes
// the source, approves the resulting startup bytes, then exercises real IPC and
// durable state across two independently started extension processes.
func TestInstalledPackageExtensionActivationJourney(t *testing.T) {
	for _, language := range []string{"go", "python"} {
		t.Run(language, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			source := t.TempDir()
			binary := filepath.Join(source, "note")
			if language == "go" {
				build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
				build.Dir = "../.."
				if out, err := build.CombinedOutput(); err != nil {
					t.Fatalf("build: %v %s", err, out)
				}
			} else {
				raw, err := os.ReadFile("../../examples/extensions/python-task-note/main.py")
				if err != nil {
					t.Fatal(err)
				}
				// Executable interpreted entrypoints must still be retained as assets.
				if err = os.WriteFile(binary, raw, 0700); err != nil {
					t.Fatal(err)
				}
			}
			content, err := os.ReadFile(binary)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(content)
			m := packages.Manifest{Schema: 1, Name: "installed-note", Version: "1.0.0", Compatibility: packages.Compatibility{MinimumHand: "1.0.0", ExtensionProtocol: 1}, Files: []packages.File{{Path: "note", Kind: "extension", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)), Executable: true}}, Extensions: []packages.Extension{{Name: "task-note", Entrypoint: "note", Capabilities: []string{"commands", "questions", "state", "context.transform"}}}}
			var runtimeApprovals []PackageRuntimeApproval
			if language == "python" {
				requirement := packages.Runtime{Name: "python", Command: "python3", MinimumVersion: "3.9.0"}
				m.Runtimes = []packages.Runtime{requirement}
				m.Extensions[0].Runtime = "python"
				review, err := packages.ReviewRuntime(ctx, requirement)
				if err != nil {
					t.Fatal(err)
				}
				digest, err := review.Digest()
				if err != nil {
					t.Fatal(err)
				}
				runtimeApprovals = []PackageRuntimeApproval{{Package: m.Name, Review: review, ApprovedDigest: digest}}
			}
			raw, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(source, packages.ManifestName), raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, pin, err := packages.VerifyDirectory(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			store, err := packages.OpenStore(filepath.Join(t.TempDir(), "packages"), "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			change, err := store.PrepareInstall(ctx, source, pin)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := change.ApprovalDigest()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.Apply(ctx, change, digest); err != nil {
				t.Fatal(err)
			}
			if err = os.RemoveAll(source); err != nil {
				t.Fatal(err)
			}
			disk := session.NewStore(t.TempDir())
			if err = disk.Create("hand", "package-journey"); err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			for round := 0; round < 2; round++ {
				func() {
					selected, err := ReviewPackageExtensions(ctx, store, []string{m.Name}, workspace, filepath.Join(t.TempDir(), "snapshots"), runtimeApprovals)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := json.Marshal(selected)
					if err != nil {
						t.Fatal(err)
					}
					file := filepath.Join(t.TempDir(), "startup.json")
					if err = os.WriteFile(file, raw, 0600); err != nil {
						t.Fatal(err)
					}
					approval := sha256.Sum256(raw)
					selected, err = ReadExtensionStartup(file, hex.EncodeToString(approval[:]))
					if err != nil {
						t.Fatal(err)
					}
					loaded, err := disk.LoadExclusive("hand", "package-journey")
					if err != nil {
						t.Fatal(err)
					}
					defer loaded.Close()
					state, err := extensions.NewStateStore(loaded, "package:installed-note:task-note")
					if err != nil {
						t.Fatal(err)
					}
					before, err := state.Get(ctx)
					if err != nil || before.Revision != round {
						t.Fatalf("reopened state: %+v %v", before, err)
					}
					controller := &Controller{Rt: &runtime.Runtime{Session: loaded}}
					host, err := controller.ActivateExtensions(ctx, selected, workspace, true)
					if err != nil {
						t.Fatal(err)
					}
					defer host.Close()
					done := make(chan error, 1)
					go func() { _, e := host.Execute(ctx, "task-note", "note", ""); done <- e }()
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					var q extensions.PendingQuestion
					for q.Token == "" {
						if pending := host.Pending(); len(pending) > 0 {
							q = pending[0]
							break
						}
						select {
						case e := <-done:
							t.Fatalf("command ended before question: %v", e)
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						case <-ticker.C:
						}
					}
					if err = host.Answer(q.Token, protocol.Answer{ID: q.Question.ID, Text: "installed package note"}); err != nil {
						t.Fatal(err)
					}
					select {
					case err = <-done:
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					saved, err := state.Get(ctx)
					if err != nil || saved.Revision != (round+1) || string(saved.Data) != `{"note":"installed package note"}` {
						t.Fatalf("saved state: %+v %v", saved, err)
					}
					if err = host.Close(); err != nil {
						t.Fatal(err)
					}
					entries, err := os.ReadDir(selected.SnapshotRoot)
					if err != nil || len(entries) != 0 {
						t.Fatalf("retired snapshots: %v %v", entries, err)
					}
				}()
			}

		})
	}
}
