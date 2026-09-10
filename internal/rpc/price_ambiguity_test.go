package rpc

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestRPCPriceTableRejectsAmbiguousRates(t *testing.T) {
	for _, rate := range []string{`"fixed_nano":100,"fixed_nano":0`, `"FIXED_NANO":0`, `"fixed_nano":null`} {
		t.Run(rate, func(t *testing.T) {
			ctx := context.Background()
			s := session.NewSession("hand", "prices")
			c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: s.ID}), Rt: &runtime.Runtime{Session: s}}
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
			payload := `{"confirmed":true,"prices":[{"provider":"local","model":"test","destination":"fixture","currency":"USD","source":"test","version":"1","effective_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z",` + rate + `}]}`
			for i := 0; i < 2; i++ {
				r := d.Dispatch(ctx, rpcRequest("ambiguous", "budget.prices.set", payload))
				if r.Error == nil {
					t.Fatal("ambiguous rates accepted")
				}
			}
			prices, err := c.CostPrices(ctx)
			if err != nil || len(prices) != 0 {
				t.Fatalf("rejected tariff changed session: %+v %v", prices, err)
			}
		})
	}
}
