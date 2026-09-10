package app

import (
	"context"
	"errors"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tokens"
)

func TestDeclaredInputCapabilitiesRejectBeforeStarting(t *testing.T) {
	calls := 0
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { calls++; return completedStream(), nil }}
	declared := []string{"text"}
	s := New(backend, Options{SessionID: "s", MaxIterations: 1, InputTypes: declared})
	declared[0] = "image"
	images := []llm.ImageContent{{MimeType: "image/png", Data: []byte{1}}}
	if _, err := s.Start(context.Background(), "image", images); err == nil {
		t.Fatal("unsupported image accepted")
	}
	if _, err := s.Execute(context.Background(), "image", images, nil); err == nil {
		t.Fatal("synchronous path bypassed input validation")
	}
	if calls != 0 || s.Snapshot().RunID != 0 || s.Snapshot().State != Idle {
		t.Fatal("rejected input started an operation")
	}
	if s.ConfigureInputs([]string{"audio"}) == nil {
		t.Fatal("unsupported input declaration accepted")
	}
	if err := s.ConfigureInputs([]string{"text", "image"}); err != nil {
		t.Fatal(err)
	}
	stream, err := s.Start(context.Background(), "image", images)
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Events {
	}
	if _, err := stream.Wait(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	_, release, err := s.reserve(context.Background(), CheckingCompletion)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.ConfigureInputs(nil), ErrBusy) {
		t.Fatal("capabilities changed during a check")
	}
	release()
}

func TestProfileSwitchUpdatesInputAdmission(t *testing.T) {
	owner := New(&backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}, Options{MaxIterations: 1})
	c := &Controller{Owner: owner, Rt: &runtime.Runtime{Provider: "local", Model: "unknown-test-model"}, BuildProfileProvider: func(config.ModelProfile) (llm.LLMProvider, error) { return &controllerProvider{}, nil }}
	if err := c.ConfigureProfiles(map[string]config.ModelProfile{"text": {Provider: "local", Model: "text", InputTypes: []string{"text"}}, "vision": {Provider: "local", Model: "vision", InputTypes: []string{"text", "image"}}}, "text"); err != nil {
		t.Fatal(err)
	}
	image := []llm.ImageContent{{MimeType: "image/png"}}
	if _, err := owner.Start(context.Background(), "work", image); err == nil {
		t.Fatal("initial profile ignored")
	}
	if err := c.SwitchProfile("vision"); err != nil {
		t.Fatal(err)
	}
	stream, err := owner.Start(context.Background(), "work", image)
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Events {
	}
	stream.Wait()
	if err := c.SwitchProfile("text"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Start(context.Background(), "work", image); err == nil {
		t.Fatal("switched profile ignored")
	}
}

func TestControllerContextMatchesRuntimeFallbackCalculation(t *testing.T) {
	rt := &runtime.Runtime{Provider: "local", Model: "unknown-test-model"}
	c := &Controller{Rt: rt}
	if got, want := c.ContextLimit(), tokens.ContextWindowFor(rt.Model, rt.ContextWindow); got != want {
		t.Fatal(got, want)
	}
	rt.ContextWindow = 24576
	if c.ContextLimit() != 24576 {
		t.Fatal("explicit context ignored")
	}
}
