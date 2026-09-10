package rpc

import (
	"encoding/json"
	"errors"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/hand/protocol"
)

func (d *Dispatcher) permission(r protocol.Request) (any, error) {
	if d.controller == nil || d.controller.PermissionState().Authority == nil {
		return nil, errors.New("scoped authority is not configured")
	}
	authority := d.controller.PermissionState().Authority
	if r.Method == "permission.revoke" {
		var p struct {
			ID string `json:"id"`
		}
		if err := decodeParams(r.Params, &p); err != nil {
			return nil, err
		}
		if p.ID == "" {
			return nil, errors.New("grant id is required")
		}
		if err := authority.Revoke(p.ID); err != nil {
			return nil, err
		}
		return map[string]bool{"revoked": true}, nil
	}
	var p struct {
		Offset int `json:"offset"`
	}
	if err := decodeParams(r.Params, &p); err != nil {
		return nil, err
	}
	grants := authority.Grants()
	if p.Offset < 0 || p.Offset > len(grants) {
		return nil, errors.New("invalid permission offset")
	}
	out := make([]permissions.ScopedGrant, 0, 16)
	size := 0
	for _, g := range grants[p.Offset:] {
		raw, err := json.Marshal(g)
		if err != nil {
			return nil, err
		}
		if len(out) == 16 || size+len(raw) > 512<<10 {
			break
		}
		out = append(out, g)
		size += len(raw)
	}
	return map[string]any{"grants": out, "next": p.Offset + len(out), "total": len(grants)}, nil
}
