package router

import (
	"reflect"
	"sync"
	"testing"

	"github.com/sausheong/goclaw/internal/channel"
	"github.com/sausheong/goclaw/internal/config"
)

func TestAcceptanceExplainSpecificity(t *testing.T) {
	rules := []config.Binding{
		{AgentID: "channel", Match: config.BindingMatch{Channel: "chat"}},
		{AgentID: "account", Match: config.BindingMatch{AccountID: "account"}},
		{AgentID: "kind", Match: config.BindingMatch{Peer: &config.PeerMatch{Kind: "direct"}}},
		{AgentID: "peer", Match: config.BindingMatch{Peer: &config.PeerMatch{ID: "sender"}}},
	}
	msg := channel.InboundMessage{Channel: "chat", AccountID: "account", ChatType: channel.ChatTypeDirect, SenderID: "sender"}
	var permute func([]config.Binding, int)
	permute = func(bindings []config.Binding, offset int) {
		if offset == len(bindings) {
			r := NewRouter(bindings, "fallback")
			decision := r.Explain(msg)
			if decision.AgentID != "peer" || decision.MatchKind != "peer.id" || decision.BindingIndex < 0 || bindings[decision.BindingIndex].AgentID != "peer" || r.Route(msg) != decision.AgentID {
				t.Fatalf("wrong priority/explanation: %+v for %+v", decision, bindings)
			}
			return
		}
		for i := offset; i < len(bindings); i++ {
			bindings[offset], bindings[i] = bindings[i], bindings[offset]
			permute(bindings, offset+1)
			bindings[offset], bindings[i] = bindings[i], bindings[offset]
		}
	}
	permute(rules, 0)
	for _, expected := range []struct{ sender, kind, account, agent, reason string }{
		{"other", "direct", "account", "kind", "peer.kind"},
		{"other", "group", "account", "account", "accountId"},
		{"other", "group", "other", "channel", "channel"},
	} {
		msg.SenderID, msg.ChatType, msg.AccountID = expected.sender, channel.ChatType(expected.kind), expected.account
		d := NewRouter(rules, "fallback").Explain(msg)
		if d.AgentID != expected.agent || d.MatchKind != expected.reason {
			t.Fatalf("unexpected decision: %+v", d)
		}
	}
}

func TestAcceptanceExplainConstraintsAndFallback(t *testing.T) {
	binding := config.Binding{AgentID: "bound", Match: config.BindingMatch{Channel: "chat", AccountID: "account", Peer: &config.PeerMatch{ID: "sender", Kind: "direct"}}}
	r := NewRouter([]config.Binding{binding}, "fallback")
	match := channel.InboundMessage{Channel: "chat", AccountID: "account", SenderID: "sender", ChatType: channel.ChatTypeDirect}
	if d := r.Explain(match); d.AgentID != "bound" || d.BindingIndex != 0 {
		t.Fatal(d)
	}
	for _, mismatch := range []channel.InboundMessage{
		{Channel: "other", AccountID: "account", SenderID: "sender", ChatType: channel.ChatTypeDirect},
		{Channel: "chat", AccountID: "other", SenderID: "sender", ChatType: channel.ChatTypeDirect},
		{Channel: "chat", AccountID: "account", SenderID: "other", ChatType: channel.ChatTypeDirect},
		{Channel: "chat", AccountID: "account", SenderID: "sender", ChatType: channel.ChatTypeGroup},
	} {
		d := r.Explain(mismatch)
		if d.AgentID != "fallback" || d.MatchKind != "fallback" || d.BindingIndex != -1 || r.Route(mismatch) != d.AgentID {
			t.Fatal(d)
		}
	}
	if d := NewRouter(nil, "empty").Explain(match); d.AgentID != "empty" || d.BindingIndex != -1 || d.MatchKind != "fallback" {
		t.Fatal(d)
	}
}

func TestAcceptanceExplainStableAndReadOnly(t *testing.T) {
	bindings := []config.Binding{{AgentID: "first", Match: config.BindingMatch{Channel: "chat"}}, {AgentID: "second", Match: config.BindingMatch{Channel: "chat"}}}
	before := append([]config.Binding(nil), bindings...)
	r := NewRouter(bindings, "fallback")
	msg := channel.InboundMessage{Channel: "chat"}
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				d := r.Explain(msg)
				if d.AgentID != "first" || d.BindingIndex != 0 || d.MatchKind != "channel" || r.Route(msg) != "first" {
					t.Errorf("unstable decision: %+v", d)
				}
				d.AgentID = "caller change"
			}
		}()
	}
	workers.Wait()
	if !reflect.DeepEqual(bindings, before) {
		t.Fatal("routing mutated caller bindings")
	}
}
