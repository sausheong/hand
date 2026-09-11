package app

import (
	"context"
	"github.com/sausheong/harness/llm"
	"net/http/httptrace"
	"time"
)

type timedProvider struct{ llm.LLMProvider }
type timedCaching struct {
	*timedProvider
	llm.PromptCachingProvider
}
type timedNonStreaming struct {
	*timedProvider
	fallback llm.NonStreamingProvider
}
type timedBoth struct {
	*timedNonStreaming
	llm.PromptCachingProvider
}

// Preserve optional interfaces: advertising a capability changes runtime behaviour.
func withProviderTiming(p llm.LLMProvider) llm.LLMProvider {
	b := &timedProvider{p}
	c, cached := p.(llm.PromptCachingProvider)
	n, fallback := p.(llm.NonStreamingProvider)
	if fallback {
		f := &timedNonStreaming{b, n}
		if cached {
			return &timedBoth{f, c}
		}
		return f
	}
	if cached {
		return &timedCaching{b, c}
	}
	return b
}
func (p *timedProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	return timeProviderCall(ctx, req, "stream", p.LLMProvider.ChatStream)
}
func (p *timedNonStreaming) ChatNonStreaming(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	return timeProviderCall(ctx, req, "non-stream", p.fallback.ChatNonStreaming)
}
func timeProviderCall(ctx context.Context, req llm.ChatRequest, mode string, call func(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error)) (<-chan llm.ChatEvent, error) {
	r := timingFrom(ctx)
	if r == nil {
		return call(ctx, req)
	}
	start := time.Now()
	r.mu.Lock()
	if len(r.report.Requests) >= 128 {
		r.report.DroppedRequests++
		r.mu.Unlock()
		return call(ctx, req)
	}
	i := len(r.report.Requests)
	r.report.Requests = append(r.report.Requests, RequestTiming{MaxOutput: req.MaxTokens, OffsetMS: milliseconds(start.Sub(r.start)), Mode: mode, Status: "running", Messages: len(req.Messages), Tools: len(req.Tools)})
	r.mu.Unlock()
	update := func(f func(*RequestTiming)) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.report.Complete {
			f(&r.report.Requests[i])
		}
	}
	mark := func(dst **float64) {
		if *dst == nil {
			v := milliseconds(time.Since(start))
			*dst = &v
		}
	}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			update(func(q *RequestTiming) {
				mark(&q.ConnectionMS)
				if q.ConnectionReused == nil {
					reused := info.Reused
					q.ConnectionReused = &reused
				}
			})
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				update(func(q *RequestTiming) { mark(&q.RequestWrittenMS) })
			}
		},
		GotFirstResponseByte: func() { update(func(q *RequestTiming) { mark(&q.ResponseHeadersMS) }) },
	})
	finish := func(status string) {
		update(func(q *RequestTiming) { q.DurationMS = milliseconds(time.Since(start)); q.Status = status })
	}
	stream, err := call(ctx, req)
	if err != nil {
		finish("failed")
		return stream, err
	}
	if stream == nil {
		finish("no stream")
		return stream, nil
	}
	out := make(chan llm.ChatEvent)
	go func() {
		status := "incomplete"
		defer close(out)
		defer func() { finish(status) }()
		for {
			select {
			case <-ctx.Done():
				status = "cancelled"
				return
			case event, ok := <-stream:
				if !ok {
					return
				}
				update(func(q *RequestTiming) {
					mark(&q.FirstEventMS)
					if event.Type == llm.EventDone {
						q.StopReason = event.StopReason
					}
					if event.Usage != nil {
						copy := *event.Usage
						q.Usage = &copy
					}
					if event.Type == llm.EventTextDelta && event.Text != "" {
						mark(&q.FirstTextMS)
					}
					if event.Type == llm.EventToolCallStart || event.Type == llm.EventToolCallDone {
						mark(&q.FirstToolMS)
					}
				})
				if event.Type == llm.EventDone && status != "failed" {
					status = "completed"
					if event.StopReason == "length" || event.StopReason == "max_tokens" {
						status = "output_limit"
					}
				}
				if event.Type == llm.EventError {
					status = "failed"
				}
				// Outer usage observers can stop consuming at the terminal event.
				// Make timing available before forwarding that event.
				if event.Type == llm.EventDone || event.Type == llm.EventError {
					finish(status)
				}
				select {
				case out <- event:
				case <-ctx.Done():
					status = "cancelled"
					return
				}
				if event.Type == llm.EventDone || event.Type == llm.EventError {
					return
				}
			}
		}
	}()
	return out, nil
}
