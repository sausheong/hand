package app

import (
	"context"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestSteeringReconciliationRestoresPendingOrConfirmsDelivery(t *testing.T) {
	for _, written := range []bool{false, true} {
		name := "not_written"
		if written {
			name = "written_without_ack"
		}
		t.Run(name, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			owner := New(nil, Options{SessionID: sess.ID})
			input, _ := owner.EnqueueInput(SteeringQueue, "correction")
			owner.active = true
			owner.state = Running
			if _, err := owner.steeringSource(context.Background()); err != nil {
				t.Fatal(err)
			}
			if written {
				entry := session.UserMessageEntry(input.Text)
				entry.ID = "steering_" + input.ID
				if err := sess.AppendContext(context.Background(), entry); err != nil {
					t.Fatal(err)
				}
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			sess, err = store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			if err := owner.reconcileSteering(sess); err != nil {
				t.Fatal(err)
			}
			queue := owner.QueuedInputs()
			if written {
				if len(queue) != 0 {
					t.Fatal("durable delivery remained queued")
				}
			} else {
				if len(queue) != 1 || queue[0].ID != input.ID || queue[0].Claimed {
					t.Fatal(queue)
				}
				if err := owner.EditQueuedInput(input.ID, "editable again"); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestSteeringReconciliationPreservesUncertainOrConflictingClaims(t *testing.T) {
	for _, mode := range []string{"closed_writer", "identity", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			owner := New(nil, Options{SessionID: sess.ID})
			input, _ := owner.EnqueueInput(SteeringQueue, "original")
			owner.active = true
			owner.state = Running
			owner.steeringSource(context.Background())
			switch mode {
			case "closed_writer":
				sess.Close()
				sess.Append(session.UserMessageEntry("failed write"))
			case "identity":
				sess = session.NewSession("hand", "other")
			case "conflict":
				entry := session.UserMessageEntry("different")
				entry.ID = "steering_" + input.ID
				sess.Append(entry)
			}
			if err := owner.reconcileSteering(sess); err == nil {
				t.Fatal("uncertain delivery accepted")
			}
			queue := owner.QueuedInputs()
			if len(queue) != 1 || !queue[0].Claimed || queue[0].Text != "original" {
				t.Fatal(queue)
			}
			if owner.RemoveQueuedInput(input.ID) == nil {
				t.Fatal("uncertain claim became removable")
			}
		})
	}
}
