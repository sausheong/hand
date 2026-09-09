// Offline wire capture of Hand's local Harness provider; no network transport.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/anthropic"
)

type captureTransport func(*http.Request) (*http.Response, error)

func (f captureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func save(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("new output directory required")
	}
	out := os.Args[1]
	if err := os.Mkdir(out, 0700); err != nil {
		return err
	}
	cases := []map[string]any{}
	for _, name := range []string{"text", "tool-roundtrip", "thinking"} {
		captures := []map[string]any{}
		http.DefaultTransport = captureTransport(func(req *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(io.LimitReader(req.Body, 16<<20+1))
			if err != nil {
				return nil, err
			}
			if len(raw) > 16<<20 {
				return nil, fmt.Errorf("oversized request")
			}
			file := fmt.Sprintf("%s-%d.json", name, len(captures)+1)
			if err = save(filepath.Join(out, file), raw); err != nil {
				return nil, err
			}
			digest := sha256.Sum256(raw)
			headers := map[string][]string{}
			for key, value := range req.Header {
				if !strings.EqualFold(key, "Authorization") && !strings.EqualFold(key, "X-Api-Key") {
					headers[key] = value
				}
			}
			captures = append(captures, map[string]any{"url": req.URL.String(), "method": req.Method, "headers": headers, "body": file, "body_sha256": hex.EncodeToString(digest[:])})
			return &http.Response{StatusCode: 400, Status: "400 Bad Request", Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"offline capture complete"}}`)), Request: req}, nil
		})
		req := llm.ChatRequest{Model: "claude-sonnet-4-5-20250929", MaxTokens: 4096, SystemPrompt: "Review code carefully.", CacheLastMessage: true,
			Messages: []llm.Message{{Role: "user", Content: "Read example.go and propose a correction."}},
			Tools:    []llm.ToolDef{{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}}}
		if name == "tool-roundtrip" {
			req.Messages = append(req.Messages, llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "tool-1", Name: "read", Input: json.RawMessage(`{"path":"example.go"}`)}}}, llm.Message{Role: "tool", ToolCallID: "tool-1", Content: "package main"})
		}
		if name == "thinking" {
			req.NativeReasoning = &llm.NativeReasoningConfig{Type: "enabled", BudgetTokens: 1024}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		events, err := anthropic.NewAnthropicProvider("offline-fixture-key", "https://api.anthropic.com").ChatStream(ctx, req)
		if err != nil {
			cancel()
			return err
		}
		errors := 0
		for event := range events {
			if event.Error != nil {
				errors++
			}
		}
		cancel()
		cases = append(cases, map[string]any{"name": name, "captures": captures, "error_events": errors})
		if len(captures) != 1 || errors != 1 {
			return fmt.Errorf("expected one captured rejected request for %s, got %d/%d", name, len(captures), errors)
		}
	}
	report := map[string]any{"status": "requests_captured", "cases": cases, "limitations": []string{"Actual local Harness provider/SDK serialize requests into an injected HTTP transport returning 400.", "No socket, full Hand agent session, actual provider call, billing or comparative quality evidence."}}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return save(filepath.Join(out, "report.json"), append(raw, '\n'))
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
