package checkpoints

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreKilledAtJournalBoundary(t *testing.T) {
	if phase := os.Getenv("HAND_RESTORE_CRASH_PHASE"); phase != "" {
		work := os.Getenv("HAND_RESTORE_CRASH_WORK")
		store, err := OpenStore(os.Getenv("HAND_RESTORE_CRASH_STORE"), work, DefaultStoreLimits())
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		before, err := store.Load(context.Background(), os.Getenv("HAND_RESTORE_CRASH_BEFORE"))
		if err != nil {
			t.Fatal(err)
		}
		after := capture(t, work)
		plan, err := PlanRestore(before, after, after, []string{"file"})
		if err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(work)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		_, err = restoreFile(context.Background(), root, plan.Actions()[0], before, func(e RestoreEvent) error {
			if phase != "applied_unrecorded" || e.Phase != "applied" {
				if err := store.RecordRestore(e); err != nil {
					return err
				}
			}
			if (phase == "prepared" && e.Phase == "prepared") || (phase != "prepared" && e.Phase == "applied") {
				fmt.Println("RESTORE_CRASH_READY")
				time.Sleep(30 * time.Second)
				return fmt.Errorf("parent did not terminate fixture")
			}
			return nil
		})
		t.Fatalf("restore fixture returned: %v", err)
	}
	for _, operation := range []string{"create", "replace", "remove"} {
		for _, phase := range []string{"prepared", "applied_unrecorded", "applied"} {
			t.Run(operation+"/"+phase, func(t *testing.T) {
				work := t.TempDir()
				if operation != "remove" {
					put(t, work, "file", "original dirty bytes", 0755)
				}
				before := capture(t, work)
				if operation == "create" {
					if err := os.Remove(filepath.Join(work, "file")); err != nil {
						t.Fatal(err)
					}
				} else {
					put(t, work, "file", "agent bytes", 0600)
				}
				after := capture(t, work)
				store, dir := newStore(t, work, DefaultStoreLimits())
				if err := store.Save(context.Background(), before); err != nil {
					t.Fatal(err)
				}
				store.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRestoreKilledAtJournalBoundary$", "-test.count=1")
				cmd.Env = append(os.Environ(), "HAND_RESTORE_CRASH_PHASE="+phase, "HAND_RESTORE_CRASH_WORK="+work, "HAND_RESTORE_CRASH_STORE="+dir, "HAND_RESTORE_CRASH_BEFORE="+before.Digest())
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
						if scan.Text() == "RESTORE_CRASH_READY" {
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
						t.Fatalf("fixture ended before barrier: %s", stderr.String())
					}
				case <-ctx.Done():
					cmd.Wait()
					t.Fatal("restore barrier timed out")
				}
				if err = cmd.Process.Kill(); err != nil {
					cmd.Wait()
					t.Fatal(err)
				}
				if err = cmd.Wait(); err == nil {
					t.Fatal("fixture was not killed")
				}
				reopened, err := OpenStore(dir, work, DefaultStoreLimits())
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				states, err := reopened.InspectRestores(context.Background())
				if err != nil || len(states) != 1 || states[0].State != phase {
					t.Fatal(states, err)
				}
				if phase == "prepared" {
					if err := reopened.ReconcileAppliedRestore(context.Background(), states[0]); err == nil {
						t.Fatal("prepared transaction reconciled as applied")
					}
					if err := reopened.CancelPreparedRestore(context.Background(), states[0]); err != nil {
						t.Fatal(err)
					}
					if err := reopened.CancelPreparedRestore(context.Background(), states[0]); err != nil {
						t.Fatal("cancellation retry failed", err)
					}
					events, err := reopened.RestoreEvents(context.Background())
					if err != nil || len(events) != 2 || events[1].Phase != "cancelled" {
						t.Fatal(events, err)
					}
					if operation != "remove" {
						data, err := os.ReadFile(filepath.Join(work, states[0].Event.RecoveryName))
						if err != nil || string(data) != "original dirty bytes" {
							t.Fatal("cancel discarded staging bytes", string(data), err)
						}
					}
				} else {
					if err := reopened.ReconcileAppliedRestore(context.Background(), states[0]); err != nil {
						t.Fatal(err)
					}
					if err := reopened.ReconcileAppliedRestore(context.Background(), states[0]); err != nil {
						t.Fatal("reconciliation retry failed", err)
					}
					events, err := reopened.RestoreEvents(context.Background())
					if err != nil || len(events) != 2 || events[1].Phase != "applied" {
						t.Fatal(events, err)
					}
				}
				if err := reopened.Close(); err != nil {
					t.Fatal(err)
				}
				resolved, err := OpenStore(dir, work, DefaultStoreLimits())
				if err != nil {
					t.Fatal(err)
				}
				defer resolved.Close()
				observed, err := resolved.InspectRestores(context.Background())
				want := "applied"
				if phase == "prepared" {
					want = "cancelled"
				}
				if err != nil || len(observed) != 1 || observed[0].State != want {
					t.Fatal("resolution not durable", observed, err)
				}
				root, err := os.OpenRoot(work)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				var target *Record
				expected := after.Records()
				restored := before.Records()
				if phase == "prepared" {
					if len(expected) > 0 {
						target = &expected[0]
					}
				} else {
					if len(restored) > 0 {
						target = &restored[0]
					}
				}
				if err = matchRestoreFile(context.Background(), root, "file", target); err != nil {
					t.Fatal("target changed during recovery", err)
				}
				if phase != "prepared" && operation != "create" {
					if err = matchRestoreFile(context.Background(), root, states[0].Event.RecoveryName, &expected[0]); err != nil {
						t.Fatal("displaced bytes lost", err)
					}
				}
			})
		}
	}
}
