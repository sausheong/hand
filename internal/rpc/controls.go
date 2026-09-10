package rpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
)

func NewControllerDispatcher(controller *app.Controller, ledger *Ledger) *Dispatcher {
	d := NewDispatcher(controller.Owner, ledger)
	d.controller = controller
	return d
}

var controllerMethods = []string{"session.list", "session.new", "session.select", "session.name", "profile.list", "profile.select"}

func (d *Dispatcher) control(ctx context.Context, r protocol.Request) (any, error) {
	c := d.controller
	if c == nil {
		return nil, errors.New("controller operations unavailable")
	}
	switch r.Method {
	case "session.list":
		var p struct {
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return nil, err
		}
		if p.Limit == 0 {
			p.Limit = 50
		}
		if p.Offset < 0 || p.Limit < 1 || p.Limit > 100 {
			return nil, errors.New("offset must be nonnegative and limit within 1..100")
		}
		sessions, err := c.ListSessionsContext(ctx)
		if err != nil {
			return nil, err
		}
		if p.Offset > len(sessions) {
			return nil, errors.New("offset exceeds session count")
		}
		end := min(len(sessions), p.Offset+p.Limit)
		// Respect the wire limit even if retained display names are large.
		size := 0
		next := p.Offset
		for next < end {
			b, err := json.Marshal(sessions[next])
			if err != nil {
				return nil, err
			}
			if size+len(b) > 512<<10 {
				break
			}
			size += len(b)
			next++
		}
		return map[string]any{"sessions": sessions[p.Offset:next], "next": next, "total": len(sessions), "current": c.SessionID()}, nil
	case "profile.list":
		return map[string]any{"profiles": c.ProfileNames(), "current": c.CurrentProfile(), "model": c.CurrentModel()}, nil
	}
	if d.active != nil {
		return nil, app.ErrBusy
	}
	switch r.Method {
	case "session.new":
		if err := decodeParams(r.Params, &struct{}{}); err != nil {
			return nil, err
		}
		if err := c.NewSessionContext(ctx); err != nil {
			return nil, err
		}
	case "session.select":
		var p struct {
			ID string `json:"id"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return nil, err
		}
		if p.ID == "" {
			return nil, errors.New("session id is required")
		}
		if err := c.ResumeSessionContext(ctx, p.ID); err != nil {
			return nil, err
		}
	case "session.name":
		var p struct {
			Name string `json:"name"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return nil, err
		}
		if err := c.RenameSessionContext(ctx, p.Name); err != nil {
			return nil, err
		}
	case "profile.select":
		var p struct {
			Name string `json:"name"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return nil, err
		}
		if p.Name == "" {
			return nil, errors.New("profile name is required")
		}
		if err := c.SwitchProfileContext(ctx, p.Name); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown controller operation")
	}
	return map[string]any{"session_id": c.SessionID(), "profile": c.CurrentProfile(), "model": c.CurrentModel()}, nil
}
