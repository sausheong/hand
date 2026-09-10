package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestRunBudgetSelectionRejectsAmbiguousPersistedRecords(t *testing.T) {
	for name, raw := range map[string]string{
		"duplicate_id":         `{"version":1,"id":"restricted","id":"roomy"}`,
		"escaped_duplicate_id": `{"version":1,"id":"restricted","\u0069d":"roomy"}`,
		"duplicate_version":    `{"version":2,"version":1,"id":"roomy"}`,
		"case_alias_id":        `{"version":1,"id":"restricted","ID":"roomy"}`,
		"case_alias_version":   `{"Version":1,"id":"roomy"}`,
		"missing_id":           `{"version":1}`,
		"missing_version":      `{"id":"roomy"}`,
		"unknown_field":        `{"version":1,"id":"roomy","other":true}`,
		"null":                 `null`,
		"array":                `[]`,
		"wrong_id_type":        `{"version":1,"id":42}`,
		"wrong_version_type":   `{"version":"1","id":"roomy"}`,
		"empty_id":             `{"version":1,"id":""}`,
		"invalid_id":           `{"version":1,"id":"../roomy"}`,
		"oversized_id":         `{"version":1,"id":"` + strings.Repeat("a", 65) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "selection"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "selection")
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			rt := &runtime.Runtime{Session: sess}
			for id, limit := range map[string]int64{"restricted": 1, "roomy": 100000} {
				if err = rt.DecideRunTokenBudget(ctx, id, limit); err != nil {
					t.Fatal(err)
				}
			}
			if err = sess.Annotate(selectedRunBudgetKind, json.RawMessage(`{"version":1,"id":"restricted"}`)); err != nil {
				t.Fatal(err)
			}
			if err = sess.Annotate(selectedRunBudgetKind, json.RawMessage(raw)); err != nil {
				t.Fatal(err)
			}
			if err = sess.Close(); err != nil {
				t.Fatal(err)
			}
			sess, err = store.LoadExclusive("hand", "selection")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			rt.Session = sess
			before := sess.Entries()
			if id, err := readSelectedRunBudget(sess); err == nil || id != "" {
				t.Errorf("ambiguous persisted selection accepted: %q %v", id, err)
			}
			backend := &HarnessBackend{Runtime: rt}
			bound, cancel, id, err := backend.PrepareSelectedRunBudget(ctx)
			cancel()
			if err == nil || id != "" || bound != ctx {
				t.Errorf("corrupt selection permitted run admission: %q %v", id, err)
			}
			if !reflect.DeepEqual(before, sess.Entries()) {
				t.Fatal("rejected selection changed saved records")
			}
		})
	}
}
