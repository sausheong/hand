package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/harness/budget"
)

func TestTerminalPriceReviewAppliesDisplayedSnapshot(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	p := budget.PriceSnapshot{Provider: "local", Model: "fixture", Destination: "http://original.example/v1", Currency: "USD", Source: "test tariff", Version: "1", EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), AllChargesBounded: true}
	path := filepath.Join(t.TempDir(), "review table.json")
	write := func() {
		raw, err := json.Marshal(map[string]any{"version": 1, "prices": []budget.PriceSnapshot{p}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if cmd := m.handleCommand("/prices-confirm nonexistent"); cmd != nil {
		t.Fatal("unreviewed confirmation dispatched")
	}
	cmd := m.handleCommand("/prices-review " + path)
	if cmd == nil {
		t.Fatal("no review worker")
	}
	msg := cmd().(sessionChangedMsg)
	if msg.err != nil || msg.priceReview == nil || !strings.Contains(strings.Join(msg.lines, "\n"), p.Destination) {
		t.Fatalf("review %+v", msg)
	}
	m.Update(msg)
	review := *m.priceReview
	p.Destination = "http://changed.example/v1"
	write()
	m.running = true
	if cmd = m.handleCommand("/prices-confirm " + review.Digest); cmd != nil {
		t.Fatal("busy install accepted")
	}
	m.running = false
	cmd = m.handleCommand("/prices-confirm " + review.Digest)
	if cmd == nil || m.priceReview != nil {
		t.Fatal("review not consumed")
	}
	msg = cmd().(sessionChangedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	installed, err := m.controller.CostPrices(context.Background())
	if err != nil || len(installed) != 1 || installed[0].Destination != "http://original.example/v1" {
		t.Fatalf("installed %+v err %v", installed, err)
	}
	if cmd = m.handleCommand("/prices-confirm " + review.Digest); cmd != nil {
		t.Fatal("reused confirmation accepted")
	}
	review.Prices[0].Destination = "http://tampered.example/v1"
	if err = m.controller.ApplyPriceReview(context.Background(), review); err == nil {
		t.Fatal("mutated reviewed content accepted")
	}
}
