//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestStateHandlerThroughPersistentPeerAndRestart(t *testing.T) {
	ctx := context.Background()
	disk := session.NewStore(t.TempDir())
	if err := disk.Create("hand", "wire-state"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		sess, err := disk.LoadExclusive("hand", "wire-state")
		if err != nil {
			t.Fatal(err)
		}
		state, err := NewStateStore(sess, "trusted-fixture-package")
		if err != nil {
			t.Fatal(err)
		}
		c := fixtureConnection(t, "state-normal")
		if _, err = c.Initialize(ctx, "fixture", []string{"commands", "state"}); err != nil {
			t.Fatal(err)
		}
		if err = c.SetCallbackHandler(state.Handle); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if _, err = c.Call(ctx, "state-write", json.RawMessage(`{"revision":0,"data":{"decision":"persistent"}}`)); err != nil {
				t.Fatal(err)
			}
		}
		raw, err := c.Call(ctx, "state", json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		var got State
		if json.Unmarshal(raw, &got) != nil || got.Revision != 1 || string(got.Data) != `{"decision":"persistent"}` {
			t.Fatal(string(raw))
		}
		if err = c.Close(); err != nil {
			t.Fatal(err)
		}
		if err = sess.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
