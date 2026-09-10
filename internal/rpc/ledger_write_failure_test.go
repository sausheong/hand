//go:build darwin || linux

package rpc

import (
	"bytes"
	"os"
	"testing"
)

func TestLedgerWriteFailurePoisonsOwnerAndPreservesDurableState(t *testing.T) {
	for _, phase := range []string{"begin", "bind", "complete"} {
		t.Run(phase, func(t *testing.T) {
			path := ledgerPath(t)
			l, err := OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			req := promptRequest()
			if phase != "begin" {
				if _, execute, err := l.Begin(req, "session"); err != nil || !execute {
					t.Fatalf("begin: %t %v", execute, err)
				}
			}
			if phase == "complete" {
				if err := l.Bind(req.ID, "run-1"); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// A real failed descriptor write exercises the OS error path without
			// simulating a successful partial write or a durable fsync.
			if err := l.file.Close(); err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "begin":
				_, execute, beginErr := l.Begin(req, "session")
				if execute || beginErr == nil {
					t.Fatal("failed intent granted execution")
				}
			case "bind":
				if err := l.Bind(req.ID, "run-1"); err == nil {
					t.Fatal("failed bind accepted")
				}
			case "complete":
				if err := l.Complete(req.ID, ledgerTerminal()); err == nil {
					t.Fatal("failed completion accepted")
				}
			}
			if l.poison == nil {
				t.Fatal("write failure left owner usable")
			}
			if _, execute, err := l.Begin(req, "session"); err == nil || execute {
				t.Fatal("poisoned owner granted retry")
			}
			other := req
			other.ID = "another-request"
			if _, execute, err := l.Begin(other, "session"); err == nil || execute {
				t.Fatal("poisoned owner granted unrelated work")
			}
			if err := l.Bind(req.ID, "run-1"); err == nil {
				t.Fatal("poisoned bind accepted")
			}
			if err := l.Complete(req.ID, ledgerTerminal()); err == nil {
				t.Fatal("poisoned completion accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed writes changed durable history")
			}
			l.Close()
			reopened, err := OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			record, execute, err := reopened.Begin(req, "session")
			if err != nil {
				t.Fatal(err)
			}
			if phase == "begin" {
				// No durable intent and no execution permission escaped; a new owner
				// may admit the request once, then must reject duplicate execution.
				if !execute || record.State != "pending" {
					t.Fatal("fresh owner could not admit unwritten intent")
				}
				if _, execute, err := reopened.Begin(req, "session"); err != nil || execute {
					t.Fatal("fresh owner admitted duplicate")
				}
			} else {
				if execute || record.State != "uncertain" || len(record.Result) != 0 {
					t.Fatalf("incomplete durable request replayed as success: %+v %t", record, execute)
				}
				if phase == "complete" && record.RunID != "run-1" {
					t.Fatal("lost accepted run identity")
				}
				if phase == "bind" && record.RunID != "" {
					t.Fatal("failed run binding became durable")
				}
			}
		})
	}
}
