package rpc

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
)

func automaticRequestID(queueID string) string { return "followup:" + queueID }

// startAutomatic runs under the dispatch mutex. Cancellation never needs that
// mutex, so attachment resolution can be interrupted during shutdown.
func (d *Dispatcher) startAutomatic() error {
	if d.autoPaused || d.closed || d.ctx.Err() != nil || d.active != nil {
		return nil
	}
	var next app.QueuedInput
	for _, input := range d.service.QueuedInputs() {
		if input.Queue == app.FollowupQueue {
			next = input
			break
		}
	}
	if next.ID == "" || !d.automatic[next.ID] {
		return nil
	}
	request := protocol.Request{Version: protocol.Version, ID: automaticRequestID(next.ID), Method: "followup.start", Params: json.RawMessage(`{}`)}
	_, execute, err := d.ledger.Begin(request, next.SessionID)
	if err != nil {
		return err
	}
	if !execute {
		return errors.New("automatic follow-up has a prior execution record; reconcile before resuming")
	}
	stream, selected, err := d.service.StartFollowup(d.ctx)
	if err != nil {
		return err
	}
	runID := d.prefix + ":" + strconv.FormatUint(stream.Identity().RunID, 10)
	if selected.ID != next.ID {
		stream.Close()
		stream.Wait()
		return errors.New("follow-up queue changed during admission")
	}
	if err = d.ledger.Bind(request.ID, runID); err != nil {
		stream.Close()
		stream.Wait()
		return err
	}
	delete(d.automatic, next.ID)
	d.active = stream
	d.wg.Add(1)
	go d.consume(stream, request.ID, runID)
	return nil
}
