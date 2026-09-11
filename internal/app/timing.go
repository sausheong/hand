package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sausheong/harness/llm"
)

// TimingReport contains durations in milliseconds, never prompts or credentials.
// Request durations may overlap (for example, child agents); they are not additive.
type TimingReport struct {
	Version         int             `json:"version"`
	RunID           uint64          `json:"run_id"`
	StartedAt       time.Time       `json:"started_at"`
	TotalMS         float64         `json:"total_ms"`
	ChecksMS        float64         `json:"checks_ms"`
	CleanupMS       float64         `json:"cleanup_ms"`
	ApprovalMS      float64         `json:"approval_wait_ms"`
	ToolSpans       []ToolTiming    `json:"tool_spans,omitempty"`
	Complete        bool            `json:"complete"`
	Requests        []RequestTiming `json:"requests"`
	DroppedRequests int             `json:"dropped_requests,omitempty"`
}

type RequestTiming struct {
	Usage      *llm.Usage `json:"usage,omitempty"`
	OffsetMS   float64    `json:"offset_ms"`
	DurationMS float64    `json:"duration_ms"`
	Mode       string     `json:"mode"`
	Status     string     `json:"status"`
	Messages   int        `json:"messages"`
	Tools      int        `json:"tools"`
	// All milestones are relative to request start. Nil means unobserved.
	FirstEventMS      *float64 `json:"first_event_ms,omitempty"`
	FirstTextMS       *float64 `json:"first_text_ms,omitempty"`
	FirstToolMS       *float64 `json:"first_tool_ms,omitempty"`
	ConnectionMS      *float64 `json:"connection_ms,omitempty"`
	RequestWrittenMS  *float64 `json:"request_written_ms,omitempty"`
	ResponseHeadersMS *float64 `json:"response_first_byte_ms,omitempty"`
	ConnectionReused  *bool    `json:"connection_reused,omitempty"`
}

type timingKey struct{}

func saveTiming(save func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("timing recorder failed")
		}
	}()
	return save()
}

type timingRecorder struct {
	mu            sync.Mutex
	start         time.Time
	report        TimingReport
	toolStarts    map[string]time.Time
	approvalStart time.Time
}

type ToolTiming struct {
	Name       string  `json:"name"`
	DurationMS float64 `json:"duration_ms"`
}

// Tool spans include approval and local event delivery, not just process time.
func (r *timingRecorder) observe(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if event.State == AwaitingApproval && r.approvalStart.IsZero() {
		r.approvalStart = time.Now()
	}
	if event.State != AwaitingApproval && !r.approvalStart.IsZero() {
		r.report.ApprovalMS += milliseconds(time.Since(r.approvalStart))
		r.approvalStart = time.Time{}
	}
	if event.Kind == "tool_call" && len(r.toolStarts)+len(r.report.ToolSpans) < 128 {
		if r.toolStarts == nil {
			r.toolStarts = map[string]time.Time{}
		}
		if _, exists := r.toolStarts[event.Details.ToolID]; !exists {
			r.toolStarts[event.Details.ToolID] = time.Now()
		}
	}
	if event.Kind == "tool_result" {
		if start, ok := r.toolStarts[event.Details.ToolID]; ok {
			name := event.Details.ToolName
			if len(name) > 64 {
				name = name[:64]
			}
			r.report.ToolSpans = append(r.report.ToolSpans, ToolTiming{Name: name, DurationMS: milliseconds(time.Since(start))})
			delete(r.toolStarts, event.Details.ToolID)
		}
	}
}

func newTiming(runID uint64) *timingRecorder {
	now := time.Now()
	return &timingRecorder{start: now, report: TimingReport{Version: 1, RunID: runID, StartedAt: now.UTC()}}
}
func milliseconds(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
func (r *timingRecorder) finish(cancelled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.report.TotalMS = milliseconds(time.Since(r.start))
	if !r.approvalStart.IsZero() {
		r.report.ApprovalMS += milliseconds(time.Since(r.approvalStart))
		r.approvalStart = time.Time{}
	}
	for i := range r.report.Requests {
		q := &r.report.Requests[i]
		if q.Status == "running" {
			q.Status = "incomplete"
			if cancelled {
				q.Status = "cancelled"
			}
			q.DurationMS = r.report.TotalMS - q.OffsetMS
		}
	}
	r.report.Complete = true
}
func (r *timingRecorder) snapshot() TimingReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	report := r.report
	if !report.Complete {
		report.TotalMS = milliseconds(time.Since(r.start))
	}
	// Deep copy pointer fields as HTTP callbacks can still be running.
	data, _ := json.Marshal(report)
	var copy TimingReport
	_ = json.Unmarshal(data, &copy)
	for i := range copy.Requests {
		if copy.Requests[i].Status == "running" {
			copy.Requests[i].DurationMS = copy.TotalMS - copy.Requests[i].OffsetMS
		}
	}
	return copy
}
func (s *Service) Timing() TimingReport {
	s.mu.Lock()
	r := s.timing
	s.mu.Unlock()
	if r == nil {
		return TimingReport{}
	}
	return r.snapshot()
}
func (b *HarnessBackend) RecordTiming(report TimingReport) error {
	if b.Runtime == nil || b.Runtime.Session == nil {
		return nil
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return b.Runtime.Session.Annotate("hand.timing", data)
}

func (r TimingReport) Summary() string {
	if r.Version == 0 {
		return "No turn timings yet."
	}
	label := "Turn"
	if !r.Complete {
		label = "Turn so far"
	}
	count := len(r.Requests) + r.DroppedRequests
	requestLabel := "model requests"
	if count == 1 {
		requestLabel = "model request"
	}
	text := fmt.Sprintf("%s %.2fs · %d %s · checks %.2fs", label, r.TotalMS/1000, count, requestLabel, r.ChecksMS/1000)
	if len(r.Requests) > 0 && r.Requests[0].FirstTextMS != nil {
		text += fmt.Sprintf(" · first text %.2fs after request", *r.Requests[0].FirstTextMS/1000)
	}
	return text
}
func (r TimingReport) Detail() string {
	if r.Version == 0 {
		return r.Summary()
	}
	var b strings.Builder
	b.WriteString(r.Summary())
	if len(r.Requests) > 0 {
		fmt.Fprintf(&b, "\nBefore first model request: %.3fs", r.Requests[0].OffsetMS/1000)
	}
	for i, q := range r.Requests {
		fmt.Fprintf(&b, "\nRequest %d (%s, %s): started +%.2fs; duration %.2fs; %d messages, %d tools available", i+1, q.Mode, q.Status, q.OffsetMS/1000, q.DurationMS/1000, q.Messages, q.Tools)
		if q.Usage != nil {
			fmt.Fprintf(&b, "\n  tokens: %d input, %d output, %d cached input", q.Usage.InputTokens, q.Usage.OutputTokens, q.Usage.CacheReadInputTokens)
		} else {
			b.WriteString("\n  tokens: not reported")
		}
		for _, v := range []struct {
			name string
			ms   *float64
		}{{"connection ready", q.ConnectionMS}, {"request sent", q.RequestWrittenMS}, {"first response byte", q.ResponseHeadersMS}, {"first event", q.FirstEventMS}, {"first text", q.FirstTextMS}, {"first tool call", q.FirstToolMS}} {
			if v.ms != nil {
				fmt.Fprintf(&b, "\n  %s: +%.3fs", v.name, *v.ms/1000)
			} else {
				fmt.Fprintf(&b, "\n  %s: not observed", v.name)
			}
		}
		if q.ConnectionReused != nil {
			fmt.Fprintf(&b, "\n  connection reused: %t", *q.ConnectionReused)
		}
		if q.RequestWrittenMS != nil && q.ResponseHeadersMS != nil && *q.ResponseHeadersMS >= *q.RequestWrittenMS {
			fmt.Fprintf(&b, "\n  request sent to first response byte: %.3fs", (*q.ResponseHeadersMS-*q.RequestWrittenMS)/1000)
		}
		if q.FirstTextMS != nil {
			fmt.Fprintf(&b, "\n  first text to stream end%s: %.3fs", func() string {
				if q.Status == "running" {
					return " (so far)"
				}
				return ""
			}(), (q.DurationMS-*q.FirstTextMS)/1000)
		}
	}
	fmt.Fprintf(&b, "\nChecks: %.3fs; cleanup: %.3fs; observed approval wait: %.3fs", r.ChecksMS/1000, r.CleanupMS/1000, r.ApprovalMS/1000)
	for _, tool := range r.ToolSpans {
		fmt.Fprintf(&b, "\nTool %s: %.3fs (includes approval and event delivery)", tool.Name, tool.DurationMS/1000)
	}
	if r.DroppedRequests > 0 {
		fmt.Fprintf(&b, "\n%d further requests omitted (detail limit).", r.DroppedRequests)
	}
	b.WriteString("\nRequest milestones are measured from each request's start. Waiting includes network, gateway and model time; Hand cannot separate the server's internal work. Stream duration includes local event delivery. Requests may overlap. Timings exclude application startup and saving this report.")
	return b.String()
}

func timingFrom(ctx context.Context) *timingRecorder {
	r, _ := ctx.Value(timingKey{}).(*timingRecorder)
	return r
}
