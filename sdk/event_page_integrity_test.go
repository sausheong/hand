package sdk

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/protocol"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPollEventsRejectsAmbiguousPage(t *testing.T) {
	event := string(snapshotFixture(t))
	for name, raw := range map[string]string{
		"missing-gap":      `{"events":[],"next":0,"latest":0}`,
		"null-gap":         `{"events":[],"next":0,"latest":0,"gap":null}`,
		"null-next":        `{"events":[],"next":null,"latest":0,"gap":false}`,
		"missing-latest":   `{"events":[],"next":0,"gap":false}`,
		"missing-events":   `{"next":0,"latest":0,"gap":false}`,
		"valid-empty":      `{"events":[],"next":0,"latest":0,"gap":false}`,
		"valid-null-list":  `{"events":null,"next":0,"latest":0,"gap":false}`,
		"duplicate-gap":    `{"events":[],"next":0,"latest":0,"gap":true,"gap":false}`,
		"gap-alias":        `{"events":[],"next":0,"latest":0,"GAP":true,"gap":false}`,
		"duplicate-next":   `{"events":[],"next":9,"next":0,"latest":0,"gap":false}`,
		"duplicate-events": `{"events":[{}],"events":[],"next":0,"latest":0,"gap":false}`,
		"cursor-duplicate": `{"events":[{"cursor":2,"cursor":1,"event":` + event + `}],"next":1,"latest":1,"gap":false}`,
		"cursor-alias":     `{"events":[{"CURSOR":2,"cursor":1,"event":` + event + `}],"next":1,"latest":1,"gap":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			server, peer := net.Pipe()
			defer server.Close()
			c := NewClient(peer)
			defer c.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				if _, err := protocol.NewReader(server).ReadFrame(); err == nil {
					protocol.NewWriter(server).Write(protocol.Response{Version: 1, RequestID: "poll", Result: json.RawMessage(raw)})
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			page, err := c.PollEvents(ctx, "poll", 0)
			if strings.HasPrefix(name, "valid-") {
				if err != nil || len(page.Events()) != 0 || page.Next() != 0 || page.Gap() {
					t.Fatal("valid empty page rejected", page, err)
				}
			} else if err == nil {
				t.Fatal("ambiguous page accepted")
			}
			<-done
		})
	}
}
