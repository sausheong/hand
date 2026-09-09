package rpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/runtime"
)

const RetainedEvents = 256
const RetainedBytes = 2 << 20

type retainedEvent struct {
	Cursor uint64         `json:"cursor"`
	Event  protocol.Event `json:"event"`
	size   int
}

// Dispatcher owns one client's application runs. Polling is bounded; terminal
// outcomes are stored in the ledger before the client can retrieve completion.
type Dispatcher struct {
	verificationCancel context.CancelFunc
	extensionCancel    context.CancelFunc
	automatic          map[string]bool
	autoPaused         bool
	controller         *app.Controller
	mu                 sync.Mutex
	service            *app.Service
	ledger             *Ledger
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	prefix             string
	negotiated, closed bool
	active             *app.Stream
	events             []retainedEvent
	cursor             uint64
	bytes              int
	failure            string
	approvals          map[string]protocol.Event
}

func NewDispatcher(service *app.Service, ledger *Ledger) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{service: service, ledger: ledger, ctx: ctx, cancel: cancel, prefix: rand.Text(), automatic: make(map[string]bool), approvals: make(map[string]protocol.Event)}
}
func decodeParams(raw json.RawMessage, out any) error {
	if err := validateParamFields(raw, out); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected one params object")
	}
	return nil
}
func (d *Dispatcher) Dispatch(ctx context.Context, r protocol.Request) (answer protocol.Response) {
	response := protocol.Response{Version: protocol.Version, RequestID: r.ID}
	fail := func(code, message string) protocol.Response {
		return protocol.Response{Version: protocol.Version, RequestID: r.ID, Error: &protocol.Error{Code: code, Message: message}}
	}
	result := func(value any) protocol.Response {
		b, err := json.Marshal(value)
		if err != nil {
			return fail("internal_error", "cannot encode result")
		}
		response.Result = b
		return response
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err.Error())
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return fail("invalid_request", "invalid envelope")
	}
	r, err = protocol.DecodeRequest(encoded)
	if err != nil {
		var wireErr *protocol.Error
		if errors.As(err, &wireErr) {
			return fail(wireErr.Code, wireErr.Message)
		}
		return fail("invalid_request", err.Error())
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fail("closed", "RPC owner is closed")
	}
	if r.Method == "hello" {
		if err := decodeParams(r.Params, &struct{}{}); err != nil {
			return fail("invalid_params", "expected empty params")
		}
		d.negotiated = true
		methods := []string{"hello", "state", "prompt", "cancel", "events.poll", "request.get", "approval.pending", "approval.respond", "steer", "followup.enqueue", "followup.start", "followup.resume", "queue.list", "queue.edit", "queue.remove"}
		if d.controller != nil {
			methods = append(methods, controllerMethods...)
			if d.controller.Extensions != nil {
				methods = append(methods, "extension.command", "extension.questions", "extension.answer", "extension.reload")
			}
			methods = append(methods, "context.state", "context.state.replace", "budget.run", "budget.run.select", "budget.run.tokens.decide", "budget.run.cost.decide", "budget.run.time.decide", "budget.time", "budget.time.decide", "budget.cost", "budget.cost.decide", "budget.prices", "budget.prices.set", "budget.tokens", "budget.tokens.decide", "summarizer.review", "summarizer.select", "summarizer.status", "context.pins", "context.pin", "context.unpin", "context.inspect", "skills.reload", "checkpoint.changes", "checkpoint.restore_preview", "checkpoint.restore", "checkpoint.recoveries", "checkpoint.recovery_resolve", "verification.profiles", "verification.check", "verification.run", "verification.list", "verification.delete")
			if d.controller.PermissionState().Authority != nil {
				methods = append(methods, "permission.list", "permission.revoke")
			}
		}
		boundary := "unrestricted host"
		if d.controller != nil && d.controller.ExecutionBoundary != "" {
			boundary = d.controller.ExecutionBoundary
		}
		return result(map[string]any{"execution_boundary": boundary, "version": protocol.Version, "methods": methods, "max_frame_bytes": protocol.MaxFrameBytes, "retained_events": RetainedEvents, "retained_bytes": RetainedBytes})
	}
	if !d.negotiated {
		return fail("not_negotiated", "send hello first")
	}
	if durableControl(r.Method) {
		record, execute, err := d.beginControl(r)
		if err != nil {
			if r.Method == "cancel" && d.ledger.unavailable() {
				if err := decodeParams(r.Params, &struct{}{}); err != nil {
					return fail("invalid_params", "expected empty params")
				}
				d.autoPaused = true
				d.cancelActive()
				d.failure = "cancellation signalled but request journal unavailable: " + err.Error()
				return fail("ledger_failure", d.failure)
			}
			return fail("request_conflict", err.Error())
		}
		if !execute {
			if record.State != "completed" {
				return fail("request_uncertain", "prior control operation requires reconciliation")
			}
			if err = json.Unmarshal(record.Result, &answer); err != nil {
				return fail("ledger_failure", "invalid stored control response")
			}
			return answer
		}
		defer func() {
			raw, err := json.Marshal(answer)
			if err == nil {
				err = d.ledger.Complete(r.ID, raw)
			}
			if err != nil {
				d.failure = "control result persistence failed: " + err.Error()
				answer = fail("ledger_failure", d.failure)
			}
		}()
	}

	switch r.Method {
	case "budget.run", "budget.run.select", "budget.run.tokens.decide", "budget.run.cost.decide", "budget.run.time.decide":
		return d.dispatchRunBudget(ctx, r, fail, result)
	case "summarizer.status":
		if d.controller == nil {
			return fail("unavailable", "summariser controller unavailable")
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		view, independent := d.controller.SelectedSummarizer()
		return result(map[string]any{"independent": independent, "selection": view})
	case "summarizer.review":
		if d.controller == nil {
			return fail("unavailable", "summariser controller unavailable")
		}
		var p app.SummarizerOptions
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		view, err := d.controller.ReviewSummarizer(ctx, p)
		if err != nil {
			return fail("summarizer_rejected", err.Error())
		}
		return result(view)
	case "summarizer.select":
		if d.controller == nil {
			return fail("unavailable", "summariser controller unavailable")
		}
		var p struct {
			Options   app.SummarizerOptions `json:"options"`
			Digest    string                `json:"digest"`
			Confirmed bool                  `json:"confirmed"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "review the summariser destination and explicitly confirm selection")
		}
		if err := d.controller.SelectSummarizer(ctx, p.Options, p.Digest); err != nil {
			return fail("summarizer_rejected", err.Error())
		}
		return result(map[string]bool{"selected": true})
	case "budget.time", "budget.time.decide":
		if d.controller == nil {
			return fail("unavailable", "budget controller unavailable")
		}
		if r.Method == "budget.time.decide" {
			var p struct {
				Seconds   int64 `json:"seconds"`
				Confirmed bool  `json:"confirmed"`
			}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if !p.Confirmed {
				return fail("confirmation_required", "explicitly confirm a new wall-clock allowance including idle time")
			}
			if err := d.controller.DecideTimeBudget(ctx, p.Seconds); err != nil {
				return fail("budget_rejected", err.Error())
			}
			return result(map[string]int64{"seconds": p.Seconds})
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		view, err := d.controller.TimeBudget(ctx)
		if err != nil {
			return fail("budget_unavailable", err.Error())
		}
		return result(view)
	case "budget.cost", "budget.cost.decide", "budget.prices", "budget.prices.set":
		if d.controller == nil {
			return fail("unavailable", "budget controller unavailable")
		}
		switch r.Method {
		case "budget.cost.decide":
			var p struct {
				Currency  string `json:"currency"`
				LimitNano int64  `json:"limit_nano"`
				Strict    *bool  `json:"strict"`
				Confirmed bool   `json:"confirmed"`
			}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if !p.Confirmed {
				return fail("confirmation_required", "explicitly confirm currency, absolute nano-unit ceiling and strict/advisory mode")
			}
			if p.Strict == nil {
				return fail("invalid_params", "strict must explicitly be true or false")
			}
			if err := d.controller.DecideCostBudget(ctx, p.Currency, p.LimitNano, *p.Strict); err != nil {
				return fail("budget_rejected", err.Error())
			}
			return result(map[string]bool{"decided": true})
		case "budget.prices.set":
			var p struct {
				Prices    json.RawMessage `json:"prices"`
				Confirmed bool            `json:"confirmed"`
			}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if !p.Confirmed {
				return fail("confirmation_required", "review and explicitly confirm route tariffs and price provenance")
			}
			prices, err := app.DecodePriceTable(p.Prices)
			if err != nil {
				return fail("invalid_params", err.Error())
			}
			if err := d.controller.SetCostPrices(ctx, prices); err != nil {
				return fail("prices_rejected", err.Error())
			}
			return result(map[string]int{"price_count": len(prices)})
		default:
			var p struct{}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
		}
		if r.Method == "budget.prices" {
			prices, err := d.controller.CostPrices(ctx)
			if err != nil {
				return fail("prices_unavailable", err.Error())
			}
			return result(prices)
		}
		view, err := d.controller.CostBudget(ctx)
		if err != nil {
			return fail("budget_unavailable", err.Error())
		}
		return result(view)
	case "budget.tokens", "budget.tokens.decide":
		if d.controller == nil {
			return fail("unavailable", "budget controller unavailable")
		}
		if r.Method == "budget.tokens.decide" {
			var p struct {
				Limit     int64 `json:"limit"`
				Confirmed bool  `json:"confirmed"`
			}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if !p.Confirmed {
				return fail("confirmation_required", "explicitly confirm the absolute session token ceiling")
			}
			if err := d.controller.DecideTokenBudget(ctx, p.Limit); err != nil {
				return fail("budget_rejected", err.Error())
			}
			return result(map[string]int64{"limit": p.Limit})
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		view, err := d.controller.TokenBudget(ctx)
		if err != nil {
			return fail("budget_unavailable", err.Error())
		}
		return result(view)

	case "context.state", "context.state.replace":
		if d.controller == nil {
			return fail("unavailable", "context controller unavailable")
		}
		if r.Method == "context.state.replace" {
			var p struct {
				Revision *int                       `json:"revision"`
				Items    []runtime.ContextStateItem `json:"items"`
			}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if p.Revision == nil || *p.Revision < 0 || p.Items == nil {
				return fail("invalid_params", "revision and items are required; use [] to clear")
			}
			if err := d.controller.SetContextState(ctx, *p.Revision, p.Items); err != nil {
				return fail("context_state_rejected", err.Error())
			}
			return result(map[string]int{"revision": *p.Revision + 1})
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		state, err := d.controller.ContextState(ctx)
		if err != nil {
			return fail("context_state_unavailable", err.Error())
		}
		return result(state)
	case "context.pins", "context.pin", "context.unpin":
		if d.controller == nil {
			return fail("unavailable", "context controller unavailable")
		}
		switch r.Method {
		case "context.pin":
			var p runtime.ContextPin
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if err := d.controller.SetContextPin(ctx, p); err != nil {
				return fail("pin_rejected", err.Error())
			}
			return result(map[string]string{"updated": p.ID})
		case "context.unpin":
			var p struct {
				ID string `json:"id"`
			}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
			if err := d.controller.RemoveContextPin(ctx, p.ID); err != nil {
				return fail("pin_rejected", err.Error())
			}
			return result(map[string]string{"removed": p.ID})
		default:
			var p struct{}
			if err := decodeParams(r.Params, &p); err != nil {
				return fail("invalid_params", err.Error())
			}
		}
		pins, err := d.controller.ContextPins(ctx)
		if err != nil {
			return fail("pins_unavailable", err.Error())
		}
		return result(pins)
	case "context.inspect":
		if d.controller == nil {
			return fail("unavailable", "context controller unavailable")
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		report, err := d.controller.InspectContext(ctx)
		if err != nil {
			return fail("context_unavailable", err.Error())
		}
		return result(report)
	case "skills.reload":
		if d.controller == nil {
			return fail("unavailable", "skill controller unavailable")
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if err := d.controller.ReloadSkills(ctx); err != nil {
			return fail("reload_rejected", err.Error())
		}
		return result(map[string]string{"reloaded": "skills"})
	case "verification.list":
		if d.controller == nil {
			return fail("unavailable", "verification controller unavailable")
		}
		var p struct {
			Offset int `json:"offset"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		page, err := d.controller.ListVerificationEvidence(ctx, p.Offset)
		if err != nil {
			return fail("verification_unavailable", err.Error())
		}
		return result(page)
	case "verification.delete":
		if d.controller == nil {
			return fail("unavailable", "verification controller unavailable")
		}
		var p struct {
			ID        string `json:"id"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicit evidence deletion confirmation required")
		}
		if err := d.controller.DeleteVerificationEvidence(ctx, p.ID); err != nil {
			return fail("verification_delete_rejected", err.Error())
		}
		return result(map[string]bool{"deleted": true})
	case "extension.command", "extension.reload":
		record, err := d.startExtension(r)
		if err != nil {
			return fail("extension_rejected", err.Error())
		}
		return result(record)
	case "extension.questions", "extension.answer":
		value, err := d.extensionControl(r)
		if err != nil {
			return fail("extension_rejected", err.Error())
		}
		return result(value)
	case "verification.run":
		record, err := d.startVerification(r)
		if err != nil {
			return fail("verification_rejected", err.Error())
		}
		return result(record)
	case "verification.profiles":
		if d.controller == nil {
			return fail("unavailable", "verification controller unavailable")
		}
		var p struct{}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		profiles, err := d.controller.VerificationProfiles(ctx)
		if err != nil {
			return fail("verification_unavailable", err.Error())
		}
		return result(profiles)
	case "verification.check":
		if d.controller == nil {
			return fail("unavailable", "verification controller unavailable")
		}
		var p struct {
			Profile string `json:"profile"`
			ID      string `json:"id"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		checked, err := d.controller.CheckSavedVerification(ctx, p.Profile, p.ID)
		if err != nil {
			return fail("verification_unavailable", err.Error())
		}
		return result(checked)
	case "checkpoint.recoveries":
		if d.controller == nil {
			return fail("unavailable", "checkpoint controller unavailable")
		}
		var p struct {
			Offset int `json:"offset"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		page, err := d.controller.CheckpointRecoveries(ctx, p.Offset)
		if err != nil {
			return fail("checkpoint_unavailable", err.Error())
		}
		return result(page)
	case "checkpoint.recovery_resolve":
		if d.controller == nil {
			return fail("unavailable", "checkpoint controller unavailable")
		}
		var p struct {
			Confirmed bool                        `json:"confirmed"`
			Action    string                      `json:"action"`
			Recovery  checkpoints.RestoreRecovery `json:"recovery"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicit recovery confirmation required")
		}
		if err := d.controller.ResolveCheckpointRecovery(ctx, p.Action, p.Recovery); err != nil {
			return fail("recovery_rejected", err.Error())
		}
		return result(map[string]bool{"resolved": true})
	case "checkpoint.restore":
		if d.controller == nil {
			return fail("unavailable", "checkpoint controller unavailable")
		}
		var p struct {
			Confirmed bool                         `json:"confirmed"`
			Preview   app.CheckpointRestorePreview `json:"preview"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		if !p.Confirmed {
			return fail("confirmation_required", "explicit confirmation of the exact restore preview is required")
		}
		files, err := d.controller.ApplyCheckpointRestore(ctx, p.Preview)
		// Preserve partial transaction results even on application failure. RPC
		// transport success does not imply a completed filesystem operation.
		value := map[string]any{"completed": err == nil, "files": files}
		if err != nil {
			value["error"] = err.Error()
		}
		return result(value)
	case "checkpoint.restore_preview":
		if d.controller == nil {
			return fail("unavailable", "checkpoint controller unavailable")
		}
		var p struct {
			RunID string   `json:"run_id"`
			Paths []string `json:"paths"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		preview, err := d.controller.PreviewCheckpointRestore(ctx, p.RunID, p.Paths)
		if err != nil {
			return fail("checkpoint_unavailable", err.Error())
		}
		return result(preview)
	case "checkpoint.changes":
		if d.controller == nil {
			return fail("unavailable", "checkpoint controller unavailable")
		}
		var p struct {
			RunID  string `json:"run_id"`
			Offset int    `json:"offset"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return fail("invalid_params", err.Error())
		}
		page, err := d.controller.CheckpointChanges(ctx, p.RunID, p.Offset)
		if err != nil {
			return fail("checkpoint_unavailable", err.Error())
		}
		return result(page)
	case "permission.list", "permission.revoke":
		value, err := d.permission(r)
		if err != nil {
			return fail("permission_rejected", err.Error())
		}
		return result(value)
	case "session.list", "session.new", "session.select", "session.name", "profile.list", "profile.select":
		value, err := d.control(ctx, r)
		if err != nil {
			return fail("operation_rejected", err.Error())
		}
		return result(value)
	case "state":
		if err := decodeParams(r.Params, &struct{}{}); err != nil {
			return fail("invalid_params", "expected empty params")
		}
		return result(map[string]any{"application": d.service.Snapshot(), "failure": d.failure, "followup_paused": d.autoPaused})
	case "cancel":
		if err := decodeParams(r.Params, &struct{}{}); err != nil {
			return fail("invalid_params", "expected empty params")
		}
		return result(map[string]bool{"requested": d.cancelActive()})
	case "steer", "followup.enqueue":
		var p struct {
			Text string `json:"text"`
			Auto bool   `json:"auto"`
		}
		if decodeParams(r.Params, &p) != nil {
			return fail("invalid_params", "expected text")
		}
		queue := app.SteeringQueue
		if r.Method == "followup.enqueue" {
			queue = app.FollowupQueue
		}
		if p.Auto && queue != app.FollowupQueue {
			return fail("invalid_params", "auto applies only to follow-ups")
		}
		input, err := d.service.EnqueueInput(queue, p.Text)
		if err != nil {
			return fail("queue_rejected", err.Error())
		}
		if p.Auto {
			d.automatic[input.ID] = true
			if err := d.startAutomatic(); err != nil {
				d.autoPaused = true
				d.failure = "automatic follow-up paused: " + err.Error()
			}
			return result(struct {
				app.QueuedInput
				RequestID string `json:"request_id"`
			}{input, automaticRequestID(input.ID)})
		}
		return result(input)
	case "queue.list":
		var p struct {
			Offset int `json:"offset"`
		}
		if decodeParams(r.Params, &p) != nil || p.Offset < 0 {
			return fail("invalid_params", "expected nonnegative offset")
		}
		inputs := d.service.QueuedInputs()
		if p.Offset > len(inputs) {
			return fail("invalid_params", "offset exceeds queue length")
		}
		next, size := p.Offset, 0
		for next < len(inputs) && next-p.Offset < 16 {
			b, _ := json.Marshal(inputs[next])
			if size+len(b) > 512<<10 {
				break
			}
			size += len(b)
			next++
		}
		return result(map[string]any{"inputs": inputs[p.Offset:next], "next": next, "total": len(inputs)})
	case "queue.edit":
		var p struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		if decodeParams(r.Params, &p) != nil {
			return fail("invalid_params", "expected id and text")
		}
		if err := d.service.EditQueuedInput(p.ID, p.Text); err != nil {
			return fail("queue_rejected", err.Error())
		}
		return result(map[string]bool{"edited": true})
	case "queue.remove":
		var p struct {
			ID string `json:"id"`
		}
		if decodeParams(r.Params, &p) != nil {
			return fail("invalid_params", "expected id")
		}
		if err := d.service.RemoveQueuedInput(p.ID); err != nil {
			return fail("queue_rejected", err.Error())
		}
		delete(d.automatic, p.ID)
		return result(map[string]bool{"removed": true})
	case "followup.resume":
		if decodeParams(r.Params, &struct{}{}) != nil {
			return fail("invalid_params", "expected empty params")
		}
		if d.active != nil {
			return fail("busy", "another RPC run is active")
		}
		d.autoPaused = false
		if err := d.startAutomatic(); err != nil {
			d.autoPaused = true
			d.failure = "automatic follow-up paused: " + err.Error()
			return fail("followup_paused", err.Error())
		}
		return result(map[string]bool{"running": d.active != nil})
	case "followup.start":
		if decodeParams(r.Params, &struct{}{}) != nil {
			return fail("invalid_params", "expected empty params")
		}
		if d.active != nil {
			if _, err := d.ledger.Lookup(r.ID); err != nil {
				return fail("busy", "another RPC run is active")
			}
		}
		if _, err := d.ledger.Lookup(r.ID); err != nil {
			for _, input := range d.service.QueuedInputs() {
				if input.Queue == app.FollowupQueue {
					if d.automatic[input.ID] {
						return fail("queue_rejected", "use followup.resume for an automatic follow-up")
					}
					break
				}
			}

		}
		record, execute, err := d.ledger.Begin(r, d.service.Snapshot().SessionID)
		if err != nil {
			return fail("request_conflict", err.Error())
		}
		if !execute {
			return result(record)
		}
		stream, _, err := d.service.StartFollowup(d.ctx)
		if err != nil {
			return fail("run_rejected", err.Error())
		}
		runID := d.prefix + ":" + strconv.FormatUint(stream.Identity().RunID, 10)
		if err = d.ledger.Bind(r.ID, runID); err != nil {
			stream.Close()
			stream.Wait()
			return fail("ledger_failure", err.Error())
		}
		d.active = stream
		d.wg.Add(1)
		go d.consume(stream, r.ID, runID)
		record, _ = d.ledger.Lookup(r.ID)
		return result(record)
	case "approval.pending":
		if err := decodeParams(r.Params, &struct{}{}); err != nil {
			return fail("invalid_params", "expected empty params")
		}
		pending := make([]protocol.Event, 0, len(d.approvals))
		for _, event := range d.approvals {
			pending = append(pending, event)
		}
		sort.Slice(pending, func(i, j int) bool { return pending[i].Sequence < pending[j].Sequence })
		out := make([]protocol.Event, 0, 16)
		size := 0
		for _, event := range pending {
			b, _ := json.Marshal(event)
			if len(out) >= 16 || size+len(b) > 512<<10 {
				break
			}
			out = append(out, event)
			size += len(b)
		}
		return result(map[string]any{"approvals": out, "remaining": len(pending) - len(out)})
	case "approval.respond":
		var p struct {
			RunID    string `json:"run_id"`
			ID       string `json:"approval_id"`
			Decision string `json:"decision"`
		}
		if decodeParams(r.Params, &p) != nil {
			return fail("invalid_params", "expected run_id, approval_id and decision")
		}
		decisions := map[string]agentio.Decision{"deny": agentio.DecisionDeny, "once": agentio.DecisionOnce, "always": agentio.DecisionAlways}
		decision, ok := decisions[p.Decision]
		if !ok {
			return fail("invalid_params", "decision must be deny, once or always")
		}
		event, ok := d.approvals[p.ID]
		if !ok || event.RunID != p.RunID || d.active == nil {
			return fail("approval_expired", "approval is not pending for that run")
		}
		if err := d.service.RespondApproval(d.active.Identity(), p.ID, decision); err != nil {
			return fail("approval_expired", err.Error())
		}
		delete(d.approvals, p.ID)
		return result(map[string]bool{"accepted": true})
	case "request.get":
		var p struct {
			ID string `json:"id"`
		}
		if decodeParams(r.Params, &p) != nil {
			return fail("invalid_params", "expected id")
		}
		record, err := d.ledger.Lookup(p.ID)
		if err != nil {
			return fail("unknown_request", err.Error())
		}
		return result(record)
	case "events.poll":
		var p struct {
			After uint64 `json:"after"`
		}
		if decodeParams(r.Params, &p) != nil {
			return fail("invalid_params", "expected after cursor")
		}
		if p.After > d.cursor {
			return fail("invalid_cursor", "cursor is ahead of this connection")
		}
		out := make([]retainedEvent, 0, 16)
		size := 0
		next := p.After
		gap := len(d.events) > 0 && p.After < d.events[0].Cursor-1
		for _, e := range d.events {
			if e.Cursor <= p.After {
				continue
			}
			if len(out) >= 16 || size+e.size > 512<<10 {
				break
			}
			out = append(out, e)
			size += e.size
			next = e.Cursor
		}
		return result(map[string]any{"events": out, "next": next, "gap": gap, "latest": d.cursor})
	case "prompt":
		var p struct {
			Text string `json:"text"`
		}
		if decodeParams(r.Params, &p) != nil || strings.TrimSpace(p.Text) == "" || len(p.Text) > 64<<10 {
			return fail("invalid_params", "text must be nonempty and at most 64 KiB")
		}
		if d.active != nil {
			if _, err := d.ledger.Lookup(r.ID); err != nil {
				return fail("busy", "another RPC run is active")
			}
		}
		record, execute, err := d.ledger.Begin(r, d.service.Snapshot().SessionID)
		if err != nil {
			return fail("request_conflict", err.Error())
		}
		if !execute {
			return result(record)
		}
		stream, err := d.service.StartInput(d.ctx, p.Text)
		if err != nil {
			return fail("run_rejected", err.Error())
		}
		runID := d.prefix + ":" + strconv.FormatUint(stream.Identity().RunID, 10)
		if err = d.ledger.Bind(r.ID, runID); err != nil {
			stream.Close()
			stream.Wait()
			return fail("ledger_failure", err.Error())
		}
		d.active = stream
		d.wg.Add(1)
		go d.consume(stream, r.ID, runID)
		record, _ = d.ledger.Lookup(r.ID)
		return result(record)
	default:
		return fail("unknown_method", "unsupported RPC method")
	}
}
func (d *Dispatcher) retain(event protocol.Event) {
	b, _ := json.Marshal(event)
	d.mu.Lock()
	defer d.mu.Unlock()
	if event.Kind == "approval_required" || event.Kind == "approval_resolved" {
		var payload struct {
			ID string `json:"approval_id"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		if event.Kind == "approval_required" {
			d.approvals[payload.ID] = event
		} else {
			delete(d.approvals, payload.ID)
		}
	}
	if event.Kind == "terminal" {
		for id, pending := range d.approvals {
			if pending.RunID == event.RunID {
				delete(d.approvals, id)
			}
		}
	}
	d.cursor++
	d.events = append(d.events, retainedEvent{Cursor: d.cursor, Event: event, size: len(b)})
	d.bytes += len(b)
	for len(d.events) > RetainedEvents || d.bytes > RetainedBytes {
		d.bytes -= d.events[0].size
		d.events[0] = retainedEvent{}
		d.events = d.events[1:]
	}
}
func (d *Dispatcher) consume(stream *app.Stream, requestID, runID string) {
	defer d.wg.Done()
	wire := app.NewWireEvents(requestID)
	for event := range stream.Events {
		e, err := wire.Encode(event)
		if err != nil {
			stream.Close()
			continue
		}
		e.RunID = runID
		d.retain(e)
	}
	outcome, _ := stream.Wait()
	final, err := wire.Encode(stream.FinalEvent())
	final.RunID = runID
	if err == nil {
		var raw []byte
		raw, err = json.Marshal(final)
		if err == nil {
			err = d.ledger.Complete(requestID, raw)
		}
	}
	if err == nil {
		d.retain(final)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, pending := range d.approvals {
		if pending.RunID == runID {
			delete(d.approvals, id)
		}
	}
	if err != nil {
		d.failure = "terminal persistence failed: " + err.Error()
	}
	if d.active == stream {
		d.active = nil
	}
	if err != nil || outcome.Status != agentio.Completed {
		d.autoPaused = true
	}
	if err == nil && outcome.Status == agentio.Completed {
		if err := d.startAutomatic(); err != nil {
			d.autoPaused = true
			d.failure = "automatic follow-up paused: " + err.Error()
		}
	}
}
func (d *Dispatcher) Close() {
	d.cancel()
	d.mu.Lock()
	d.closed = true
	d.cancel()
	d.mu.Unlock()
	d.wg.Wait()
}

// Caller holds d.mu. Signalling cancellation must not wait for journal I/O.
func (d *Dispatcher) cancelActive() bool {
	requested := d.service.Cancel()
	if d.extensionCancel != nil {
		d.extensionCancel()
		requested = true
	}
	if d.verificationCancel != nil {
		d.verificationCancel()
		requested = true
	}
	return requested
}
