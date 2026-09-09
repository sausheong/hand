//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/extension/protocol"
	"testing"
)

func TestQuestionCallbackUsesLiveHostToken(t *testing.T) {
	c := fixtureConnection(t, "question")
	if _, err := c.Initialize(context.Background(), "fixture", []string{"commands", "questions"}); err != nil {
		t.Fatal(err)
	}
	broker := NewQuestions()
	defer broker.Close()
	if err := c.SetCallbackHandler(broker.Handler("fixture")); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		raw, err := c.Call(context.Background(), "ask", json.RawMessage(`{}`))
		if err == nil {
			var answer protocol.Answer
			if json.Unmarshal(raw, &answer) != nil || answer.Choice != "one" {
				t.Error("answer not delivered", string(raw))
			}
		}
		done <- err
	}()
	pending := waitQuestions(t, broker, 1)[0]
	if pending.Extension != "fixture" {
		t.Fatal(pending)
	}
	if err := broker.Respond(pending.Token, protocol.Answer{ID: "choose", Choice: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(broker.Pending()) != 0 {
		t.Fatal("completed question retained")
	}
}
