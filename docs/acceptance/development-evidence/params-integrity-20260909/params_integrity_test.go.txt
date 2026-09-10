package rpc

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestBudgetConfirmationRejectsAmbiguousParams(t *testing.T) {
	for _, body := range []string{
		`{"limit":2000,"confirmed":false,"confirmed":true}`,
		`{"limit":2000,"Confirmed":true}`,
		`{"limit":2000,"confirmed":false,"confi\u0072med":true}`,
		`{"limit":1,"limit":2000,"confirmed":true}`,
	} {
		t.Run(body, func(t *testing.T) {
			ctx := context.Background()
			sess := session.NewSession("hand", "params")
			c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: sess.ID}), Rt: &runtime.Runtime{Session: sess}}
			l, err := OpenLedger(ledgerPath(t))
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			d := NewControllerDispatcher(c, l)
			defer d.Close()
			if r := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); r.Error != nil {
				t.Fatal(r.Error)
			}
			if r := d.Dispatch(ctx, rpcRequest("initial", "budget.tokens.decide", `{"limit":1000,"confirmed":true}`)); r.Error != nil {
				t.Fatal(r.Error)
			}
			for i := 0; i < 2; i++ {
				if r := d.Dispatch(ctx, rpcRequest("ambiguous", "budget.tokens.decide", body)); r.Error == nil {
					t.Fatal("ambiguous confirmation accepted")
				}
			}
			state, err := c.TokenBudget(ctx)
			if err != nil || state.Limit != 1000 {
				t.Fatalf("invalid request changed limit: %+v %v", state, err)
			}
		})
	}
}
