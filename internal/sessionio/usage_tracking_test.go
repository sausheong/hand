package sessionio

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

func TestUsageTrackingPreservesLegacyUncertaintyAcrossRestart(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "fresh"
		if legacy {
			name = "legacy"
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
			if legacy {
				sess.Append(session.UserMessageEntry("old question"))
				sess.Append(session.AssistantMessageEntry("old answer"))
			}
			summary, err := ReadUsage(sess)
			if err != nil || summary.PriorUsageUnknown != legacy {
				t.Fatal(summary, err)
			}
			if err := BeginUsageTracking(sess); err != nil {
				t.Fatal(err)
			}
			sess.Append(session.UserMessageEntry("new question"))
			request := llm.RequestUsage{ID: "one", Model: "m", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 10}}
			if err := RecordRequestUsage(sess, request); err != nil {
				t.Fatal(err)
			}
			if err := BeginUsageTracking(sess); err != nil {
				t.Fatal(err)
			}
			if len(sess.Annotations(usageTrackingKind)) != 1 {
				t.Fatal("origin duplicated")
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			sess, err = store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			summary, err = ReadUsage(sess)
			if err != nil || summary.PriorUsageUnknown != legacy || summary.Requests != 1 || summary.Total.InputTokens != 10 || summary.Unknown != 0 {
				t.Fatal(summary, err)
			}
		})
	}
}

func TestForkAccountingExcludesInheritedConsumption(t *testing.T) {
	manager, err := NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Fork(context.Background(), []session.SessionEntry{session.UserMessageEntry("inherited question"), session.AssistantMessageEntry("inherited answer")})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Session.Close()
	summary, err := ReadUsage(result.Session)
	if err != nil || summary.PriorUsageUnknown || summary.Requests != 0 {
		t.Fatal(summary, err)
	}
}

func TestUsageTrackingRejectsFutureAndConflictingOrigins(t *testing.T) {
	for _, payload := range []string{`{"version":2,"prior_usage_unknown":false}`, `{"version":1}`, `{"version":1,"prior_usage_unknown":true}`} {
		sess := session.NewSession("hand", "key")
		if err := BeginUsageTracking(sess); err != nil {
			t.Fatal(err)
		}
		if err := sess.Annotate(usageTrackingKind, json.RawMessage(payload)); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadUsage(sess); err == nil {
			t.Fatalf("accepted %s", payload)
		}
		if err := BeginUsageTracking(sess); err == nil {
			t.Fatalf("overwrote %s", payload)
		}
	}
}
