package sessionio

import (
	"encoding/json"
	"testing"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

func TestUsageSummaryPersistsUnknownAndDeduplicates(t *testing.T) {
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	known := llm.RequestUsage{ID: "one", Model: "model", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 100, OutputTokens: 5, CacheReadInputTokens: 70}}
	unknown := llm.RequestUsage{ID: "two", Model: "model", Status: "failed", Category: llm.CallRetry, Source: "unavailable"}
	for _, r := range []llm.RequestUsage{known, unknown, known} {
		if err := RecordRequestUsage(sess, r); err != nil {
			t.Fatal(err)
		}
	}
	if len(sess.Annotations(usageAnnotationKind)) != 2 {
		t.Fatal("identical retry added a durable duplicate")
	}
	if sess.LeafID() != "" {
		t.Fatal("accounting created conversation leaf")
	}
	sess.Close()
	sess, err = store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	summary, err := ReadUsage(sess)
	if err != nil || summary.Requests != 2 || summary.Unknown != 1 || summary.Total.InputTokens != 100 || summary.Total.OutputTokens != 5 || summary.Total.CacheReadInputTokens != 70 {
		t.Fatal(summary, err)
	}
	known.Usage.OutputTokens++
	before := len(sess.Entries())
	if err := RecordRequestUsage(sess, known); err == nil {
		t.Fatal("conflicting request accepted")
	}
	if len(sess.Entries()) != before {
		t.Fatal("conflicting request changed journal")
	}
	if summary, err := ReadUsage(sess); err != nil || summary.Total.OutputTokens != 5 {
		t.Fatal("conflict poisoned existing accounting", summary, err)
	}
}

func TestUsageRejectsInvalidAndFutureRecords(t *testing.T) {
	sess := session.NewSession("hand", "key")
	invalid := llm.RequestUsage{ID: "x", Model: "m", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 1, CacheReadInputTokens: 2}}
	if err := RecordRequestUsage(sess, invalid); err == nil {
		t.Fatal("invalid cache accounting accepted")
	}
	if len(sess.Entries()) != 0 {
		t.Fatal("invalid usage persisted")
	}
	if err := sess.Annotate(usageAnnotationKind, json.RawMessage(`{"version":2,"request":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadUsage(sess); err == nil {
		t.Fatal("future usage schema accepted")
	}
}
