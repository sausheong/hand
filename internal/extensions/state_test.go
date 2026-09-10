package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sausheong/harness/session"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestExtensionStateRestartIsolationAndRevision(t *testing.T) {
	ctx := context.Background()
	disk := session.NewStore(t.TempDir())
	if err := disk.Create("hand", "state"); err != nil {
		t.Fatal(err)
	}
	sess, err := disk.LoadExclusive("hand", "state")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sess.Close() }()
	a, err := NewStateStore(sess, "publisher/package-a")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Set(ctx, 0, json.RawMessage(`{"decision":"keep exact"}`)); err != nil {
		t.Fatal(err)
	}
	if err = a.Set(ctx, 0, json.RawMessage(`{}`)); !errors.Is(err, session.ErrAnnotationConflict) {
		t.Fatal(err)
	}
	b, _ := NewStateStore(sess, "publisher/package-b")
	other, err := b.Get(ctx)
	if err != nil || other.Revision != 0 || string(other.Data) != "{}" {
		t.Fatal("namespace leaked", other, err)
	}
	sess.Compact("summary ignores extension state", 0)
	if err = sess.Flush(); err != nil {
		t.Fatal(err)
	}
	if err = sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = disk.LoadExclusive("hand", "state")
	if err != nil {
		t.Fatal(err)
	}
	a, _ = NewStateStore(sess, "publisher/package-a")
	got, err := a.Get(ctx)
	if err != nil || got.Revision != 1 || string(got.Data) != `{"decision":"keep exact"}` {
		t.Fatal(got, err)
	}
	if _, e := a.Handle(ctx, "state.get", json.RawMessage(`{"identity":"publisher/package-b"}`)); e == nil {
		t.Fatal("namespace selection accepted")
	}
	if _, e := a.Handle(ctx, "file.read", json.RawMessage(`{}`)); e == nil {
		t.Fatal("state grants resource access")
	}
	if err = a.Set(ctx, 1, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	got, err = a.Get(ctx)
	if err != nil || got.Revision != 2 || string(got.Data) != "{}" {
		t.Fatal(got, err)
	}
}
func TestExtensionStateConcurrentUpdatesAndCorruption(t *testing.T) {
	ctx := context.Background()
	sess := session.NewSession("hand", "race")
	a, _ := NewStateStore(sess, "same-owner")
	b, _ := NewStateStore(sess, "same-owner")
	var successes, conflicts atomic.Int32
	var wg sync.WaitGroup
	for _, s := range []*StateStore{a, b} {
		wg.Add(1)
		go func(s *StateStore) {
			defer wg.Done()
			err := s.Set(ctx, 0, json.RawMessage(`{"x":1}`))
			if err == nil {
				successes.Add(1)
			} else if errors.Is(err, session.ErrAnnotationConflict) {
				conflicts.Add(1)
			} else {
				t.Error(err)
			}
		}(s)
	}
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatal(successes.Load(), conflicts.Load())
	}
	for _, raw := range []string{`null`, `[]`, `{"x":1,"x":2}`, `{"x":"` + strings.Repeat("x", MaxStateBytes) + `"}`} {
		if err := a.Set(ctx, 1, json.RawMessage(raw)); err == nil {
			t.Fatal("invalid state persisted")
		}
	}
	// JSON escaping must not persist data larger than the reader accepts.
	expanding := json.RawMessage(`{"x":"` + strings.Repeat("x", MaxStateBytes-10) + `<"}`)
	if err := a.Set(ctx, 1, expanding); err == nil {
		t.Fatal("encoded oversize state persisted")
	}
	if len(sess.Annotations(a.kind)) != 1 {
		t.Fatal("invalid update changed journal")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.Set(cancelled, 1, json.RawMessage(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := sess.Annotate(a.kind, json.RawMessage(`{"version":99,"data":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get(ctx); err == nil {
		t.Fatal("corrupt state ignored")
	}
	if err := a.Set(ctx, 2, json.RawMessage(`{}`)); err == nil {
		t.Fatal("corrupt state overwritten")
	}
}
func TestExtensionStateRevisionQuota(t *testing.T) {
	sess := session.NewSession("hand", "quota")
	s, _ := NewStateStore(sess, "bounded")
	ctx := context.Background()
	for i := 0; i < MaxStateRevisions; i++ {
		if err := s.Set(ctx, i, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Set(ctx, MaxStateRevisions, json.RawMessage(`{"new":true}`)); err == nil {
		t.Fatal("unbounded state journal")
	}
	got, err := s.Get(ctx)
	if err != nil || got.Revision != MaxStateRevisions || string(got.Data) != "{}" {
		t.Fatal(got, err)
	}
}
