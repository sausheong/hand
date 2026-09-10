// Exercise the actual local Harness provider over a guarded loopback transport.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/anthropic"
)

type guard struct {
	base *url.URL
	next http.RoundTripper
}

func (g guard) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != g.base.Scheme || r.URL.Host != g.base.Host || r.URL.Path != "/v1/messages" {
		return nil, fmt.Errorf("non-gateway request forbidden")
	}
	return g.next.RoundTrip(r)
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("configuration and new report path required")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var config struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return err
	}
	base, err := url.Parse(config.URL)
	if err != nil {
		return err
	}
	if base.Scheme != "http" || base.Hostname() != "127.0.0.1" || base.User != nil {
		return fmt.Errorf("loopback gateway required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	http.DefaultTransport = guard{base, transport}
	cases := []map[string]any{}
	for _, name := range []string{"text", "tool-roundtrip", "thinking"} {
		req := llm.ChatRequest{Model: "claude-sonnet-4-5-20250929", MaxTokens: 4096, SystemPrompt: "Review code carefully.", CacheLastMessage: true,
			Messages: []llm.Message{{Role: "user", Content: "Read example.go."}},
			Tools:    []llm.ToolDef{{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}}}
		if name == "tool-roundtrip" {
			req.Messages = append(req.Messages, llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "tool-1", Name: "read", Input: json.RawMessage(`{"path":"example.go"}`)}}}, llm.Message{Role: "tool", ToolCallID: "tool-1", Content: "package main"})
		}
		if name == "thinking" {
			req.NativeReasoning = &llm.NativeReasoningConfig{Type: "enabled", BudgetTokens: 1024}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		events, err := anthropic.NewAnthropicProvider(config.Token, config.URL).ChatStream(ctx, req)
		if err != nil {
			cancel()
			return err
		}
		var text strings.Builder
		done, errors := 0, 0
		var usage *llm.Usage
		stop := ""
		types := []int{}
		for event := range events {
			types = append(types, int(event.Type))
			if event.Type == llm.EventTextDelta {
				text.WriteString(event.Text)
			}
			if event.Type == llm.EventDone {
				done++
				usage = event.Usage
				stop = event.StopReason
			}
			if event.Error != nil {
				errors++
			}
		}
		cancel()
		cases = append(cases, map[string]any{"name": name, "event_types": types, "text": text.String(), "done_events": done, "error_events": errors, "usage": usage, "stop_reason": stop})
		if done != 1 || errors != 0 || usage == nil || usage.InputTokens != 5 || usage.OutputTokens != 2 || stop != "end_turn" || text.String() != "offline gateway fixture" {
			return fmt.Errorf("incomplete or unexpected fixture response for %s", name)
		}
	}
	report := map[string]any{"status": "sdk_gateway_passed", "go_version": runtime.Version(), "cases": cases}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(os.Args[2], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
