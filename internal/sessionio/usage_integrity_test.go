package sessionio

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

func TestUsageAmbiguousFieldsRejectedAfterRestartWithoutMutation(t *testing.T) {
	request := llm.RequestUsage{ID: "one", Model: "m", Status: "completed", Category: llm.CallGeneration, Source: "reported", Usage: &llm.Usage{InputTokens: 100, OutputTokens: 7}}
	encoded, err := json.Marshal(usageRecord{Version: 1, Request: request})
	if err != nil {
		t.Fatal(err)
	}
	base := string(encoded)
	cases := []struct{ name, kind, payload string }{
		{"duplicate-input", usageAnnotationKind, strings.Replace(base, `"input_tokens":100`, `"input_tokens":100,"input_tokens":1`, 1)},
		{"escaped-duplicate", usageAnnotationKind, strings.Replace(base, `"input_tokens":100`, `"input_tokens":100,"input_\u0074okens":1`, 1)},
		{"input-case-alias", usageAnnotationKind, strings.Replace(base, `"input_tokens":100`, `"input_tokens":100,"INPUT_TOKENS":1`, 1)},
		{"null-input", usageAnnotationKind, strings.Replace(base, `"input_tokens":100`, `"input_tokens":null`, 1)},
		{"missing-input", usageAnnotationKind, strings.Replace(base, `"input_tokens":100,`, ``, 1)},
		{"null-cache", usageAnnotationKind, strings.Replace(base, `"output_tokens":7`, `"output_tokens":7,"cache_read_input_tokens":null`, 1)},
		{"request-case-alias", usageAnnotationKind, strings.Replace(base, `"request_id":"one"`, `"request_id":"one","REQUEST_ID":"other"`, 1)},
		{"duplicate-version", usageAnnotationKind, strings.Replace(base, `"version":1`, `"version":2,"version":1`, 1)},
		{"unknown-record-field", usageAnnotationKind, strings.Replace(base, `"version":1`, `"version":1,"override":true`, 1)},
		{"duplicate-origin", usageTrackingKind, `{"version":1,"prior_usage_unknown":true,"prior_usage_unknown":false}`},
		{"origin-case-alias", usageTrackingKind, `{"version":1,"prior_usage_unknown":true,"PRIOR_USAGE_UNKNOWN":false}`},
		{"unknown-origin-field", usageTrackingKind, `{"version":1,"prior_usage_unknown":false,"override":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := session.NewStore(root)
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			if err := sess.Annotate(tc.kind, json.RawMessage(tc.payload)); err != nil {
				t.Fatal(err)
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "hand", "key.jsonl")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sess, err = store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			if summary, err := ReadUsage(sess); err == nil {
				t.Fatalf("ambiguous accounting accepted: %+v", summary)
			}
			next := request
			next.ID = "next"
			if err := RecordRequestUsage(sess, next); err == nil {
				t.Fatal("appended after corrupt accounting")
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("corrupt accounting bytes changed", err)
			}
		})
	}
}
