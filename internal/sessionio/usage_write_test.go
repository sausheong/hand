package sessionio

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

func TestUsageWriteRejectsOverflowWithoutChangingDurableJournal(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	if err := store.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	request := llm.RequestUsage{ID: "first", Model: "model", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: int(^uint(0) >> 1)}}
	if err := RecordRequestUsage(sess, request); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hand", "key.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordRequestUsage(sess, request); err != nil {
		t.Fatal(err)
	}
	request.ID = "overflow"
	request.Usage = &llm.Usage{InputTokens: 1}
	if err := RecordRequestUsage(sess, request); err == nil {
		t.Fatal("overflow accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("retry or rejected overflow changed durable bytes")
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	summary, err := ReadUsage(sess)
	if err != nil || summary.Requests != 1 || summary.Total.InputTokens != int(^uint(0)>>1) {
		t.Fatal(summary, err)
	}
}

func TestUsageWriteRefusesInvalidExistingJournal(t *testing.T) {
	request := llm.RequestUsage{ID: "one", Model: "model", Status: "failed", Category: llm.CallGeneration, Source: "unavailable"}
	for _, kind := range []string{"future", "malformed", "conflicting"} {
		t.Run(kind, func(t *testing.T) {
			sess := session.NewSession("hand", "key")
			payload := json.RawMessage(`{"version":2,"request":{}}`)
			if kind == "malformed" {
				payload = json.RawMessage(`{"version":1,"request":{}}`)
			}
			if kind == "conflicting" {
				if err := RecordRequestUsage(sess, request); err != nil {
					t.Fatal(err)
				}
				changed := request
				changed.Model = "other"
				payload, _ = json.Marshal(usageRecord{Version: 1, Request: changed})
			}
			if err := sess.Annotate(usageAnnotationKind, payload); err != nil {
				t.Fatal(err)
			}
			before := len(sess.Entries())
			next := request
			next.ID = "two"
			if err := RecordRequestUsage(sess, next); err == nil {
				t.Fatal("appended to invalid journal")
			}
			if len(sess.Entries()) != before {
				t.Fatal("changed invalid journal")
			}
			if _, err := ReadUsage(sess); err == nil {
				t.Fatal("reader accepted invalid journal")
			}
		})
	}
}
