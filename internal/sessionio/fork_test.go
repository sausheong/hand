package sessionio

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sausheong/harness/session"
)

func TestForkFailureDoesNotPublishPartialCopy(t *testing.T) {
	root := t.TempDir()
	m, err := NewManager(root, t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	source, err := m.Create(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Session.Close()
	before, err := m.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	entry := session.UserMessageEntry("valid prefix")
	entry.ID = "same"
	if _, err := m.Fork(context.Background(), []session.SessionEntry{entry, entry}); err == nil {
		t.Fatal("invalid duplicate graph published")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Fork(ctx, nil); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := m.Catalogue().Snapshot()
	if err != nil || len(after.Sessions) != len(before.Sessions) || after.LastActiveID != before.LastActiveID {
		t.Fatal("failed fork changed catalogue", err)
	}
	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".fork-") {
			t.Fatal("staging directory leaked")
		}
	}
}

func TestForkBoundaryErrorsPreserveSourceAndRecoverCompleteOrphan(t *testing.T) {
	for _, stage := range []string{"copied_entry", "before_publish", "after_publish", "before_catalogue"} {
		t.Run(stage, func(t *testing.T) {
			m, err := NewManager(t.TempDir(), t.TempDir(), "hand")
			if err != nil {
				t.Fatal(err)
			}
			source, err := m.Create(context.Background(), "source")
			if err != nil {
				t.Fatal(err)
			}
			defer source.Session.Close()
			failure := errors.New("injected fork boundary failure")
			m.forkCheckpoint = func(at string) error {
				if at == stage {
					return failure
				}
				return nil
			}
			if _, err := m.Fork(context.Background(), []session.SessionEntry{session.UserMessageEntry("complete copy")}); !errors.Is(err, failure) {
				t.Fatalf("failure lost: %v", err)
			}
			snapshot, err := m.Catalogue().Snapshot()
			if err != nil || len(snapshot.Sessions) != 1 || snapshot.LastActiveID != source.Session.ID {
				t.Fatal("failed fork selected new session", err)
			}
			if _, err := m.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			snapshot, err = m.Catalogue().Snapshot()
			expected := 1
			if stage == "after_publish" || stage == "before_catalogue" {
				expected = 2
			}
			if err != nil || len(snapshot.Sessions) != expected || snapshot.LastActiveID != source.Session.ID {
				t.Fatal("wrong recovery boundary", snapshot, err)
			}
		})
	}
}
