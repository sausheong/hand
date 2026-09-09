package app

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/session"
)

func TestSteeringSnapshotSurvivesFileChangesAndReconciles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("original pixels"), 0600)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("original note"), 0600)
	store := session.NewStore(t.TempDir())
	store.Create("hand", "key")
	sess, err := store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	owner := New(nil, Options{SessionID: sess.ID, ResolveInput: func(ctx context.Context, text string) (agentio.PromptInput, error) {
		return agentio.ParsePromptInput(ctx, dir, text)
	}})
	input, _ := owner.EnqueueInput(SteeringQueue, "inspect image.png @note.txt")
	owner.active = true
	owner.state = Running
	message, err := owner.steeringSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Images) != 1 || !strings.Contains(message.Text, "original note") {
		t.Fatal(message)
	}
	message.Images[0].Data[0] = 'X'
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("changed pixels"), 0600)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("changed note"), 0600)
	retry, err := owner.steeringSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(retry.Images[0].Data) != "original pixels" || strings.Contains(retry.Text, "changed note") {
		t.Fatal("snapshot mutated", retry)
	}
	entry := session.UserMessageWithImagesEntry(retry.Text, []session.ImageData{{MimeType: retry.Images[0].MimeType, Data: base64.StdEncoding.EncodeToString(retry.Images[0].Data)}})
	entry.ID = "steering_" + input.ID
	if err := sess.AppendContext(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if err := owner.reconcileSteering(sess); err != nil {
		t.Fatal(err)
	}
	if len(owner.QueuedInputs()) != 0 || len(owner.steeringSnapshots) != 0 {
		t.Fatal("delivered snapshot retained")
	}
}

func TestSteeringAttachmentFailureLeavesEditableQueue(t *testing.T) {
	dir := t.TempDir()
	owner := New(nil, Options{InputTypes: []string{"text"}, ResolveInput: func(ctx context.Context, text string) (agentio.PromptInput, error) {
		return agentio.ParsePromptInput(ctx, dir, text)
	}})
	input, _ := owner.EnqueueInput(SteeringQueue, "missing.png")
	owner.active = true
	owner.state = Running
	for _, exists := range []bool{false, true} {
		if exists {
			os.WriteFile(filepath.Join(dir, "missing.png"), []byte("pixels"), 0600)
		}
		if _, err := owner.steeringSource(context.Background()); err == nil {
			t.Fatal("bad steering attachment admitted")
		}
		queue := owner.QueuedInputs()
		if len(queue) != 1 || queue[0].Claimed || len(owner.steeringSnapshots) != 0 {
			t.Fatal(queue)
		}
	}
	if err := owner.EditQueuedInput(input.ID, "plain correction"); err != nil {
		t.Fatal(err)
	}
}
