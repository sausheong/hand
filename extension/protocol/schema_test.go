package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHelloCannotExpandHostCapabilities(t *testing.T) {
	h := Hello{Version: 1, Name: "example", Capabilities: []string{"commands", "lifecycle"}, Commands: []Command{{Name: "inspect", Description: "Inspect task"}}, Subscriptions: []string{"run.finish"}}
	if err := h.Validate([]string{"commands", "lifecycle"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Validate([]string{"commands"}); err == nil {
		t.Fatal("capability expansion accepted")
	}
	h.Capabilities = append(h.Capabilities, "policy.override")
	if err := h.Validate(h.Capabilities); err == nil {
		t.Fatal("unknown authority accepted")
	}
	h.Capabilities = []string{"commands", "lifecycle"}
	h.Commands = append(h.Commands, h.Commands[0])
	if err := h.Validate(h.Capabilities); err == nil {
		t.Fatal("duplicate command accepted")
	}
}
func TestDeclarativePresentationRejectsTerminalAuthority(t *testing.T) {
	p := Presentation{Blocks: []Block{{Kind: "text", Text: "Progress note"}, {Kind: "code", Language: "go", Text: "fmt.Println(1)\n"}, {Kind: "list", Items: []string{"one", "two"}}}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"\x1b[2J", "\x9b31m", "\u202eoverride", "\x00", "\a"} {
		p := Presentation{Blocks: []Block{{Kind: "text", Text: s}}}
		if err := p.Validate(); err == nil {
			t.Fatal("terminal control accepted", s)
		}
	}
	for _, raw := range []string{`{"blocks":[{"kind":"text","text":"x","outcome":"completed"}]}`, `{"blocks":[],"permissions":["all"]}`, `{"blocks":[],"blocks":[]}`} {
		var p Presentation
		if err := DecodePayload(json.RawMessage(raw), &p); err == nil {
			t.Fatal("unknown authority or duplicate field accepted")
		}
	}
	p.Blocks = []Block{{Kind: "outcome", Text: "completed"}}
	if err := p.Validate(); err == nil {
		t.Fatal("outcome override accepted")
	}
	p.Blocks = []Block{{Kind: "text", Text: strings.Repeat("x", 16385)}}
	if err := p.Validate(); err == nil {
		t.Fatal("oversized text accepted")
	}
}
func TestQuestionAnswersAreExplicitAndBound(t *testing.T) {
	q := Question{ID: "question-1", Title: "Choose mode", Options: []Option{{ID: "safe", Label: "Safe mode"}}, AllowFreeText: true}
	for _, a := range []Answer{{ID: q.ID, Choice: "safe"}, {ID: q.ID, Text: "custom"}, {ID: q.ID, Cancelled: true}} {
		if err := q.ValidateAnswer(a); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []Answer{{ID: "other", Choice: "safe"}, {ID: q.ID}, {ID: q.ID, Choice: "unknown"}, {ID: q.ID, Choice: "safe", Text: "both"}, {ID: q.ID, Cancelled: true, Choice: "safe"}} {
		if err := q.ValidateAnswer(a); err == nil {
			t.Fatal("invalid answer accepted", a)
		}
	}
	q.AllowFreeText = false
	if err := q.ValidateAnswer(Answer{ID: q.ID, Text: "text"}); err == nil {
		t.Fatal("unoffered answer accepted")
	}
}
