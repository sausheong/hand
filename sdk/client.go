// Package sdk provides the versioned Hand RPC client. This API is experimental
// until its released-package compatibility acceptance gate passes.
package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/sausheong/hand/protocol"
)

// Client owns a duplex connection. Close must unblock both Read and Write on
// the supplied connection. Calls are serialized; callers may use it concurrently.
// Cancelling an in-flight call closes the connection because its execution may
// already have started. Reconnect and query the durable request ID to reconcile.
type Client struct {
	conn     io.ReadWriteCloser
	reader   *protocol.Reader
	writer   *protocol.Writer
	gate     chan struct{}
	once     sync.Once
	done     chan struct{}
	closeErr error
}

// NewClient transfers ownership of conn. Negotiate with Hello before other calls.
func NewClient(conn io.ReadWriteCloser) *Client {
	return &Client{conn: conn, reader: protocol.NewReader(conn), writer: protocol.NewWriter(conn), gate: make(chan struct{}, 1), done: make(chan struct{})}
}

// Close interrupts pending I/O. It is safe to call repeatedly and concurrently.
func (c *Client) Close() error {
	c.once.Do(func() { c.closeErr = c.conn.Close(); close(c.done) })
	return c.closeErr
}

// Call sends a request and returns an independently owned result. A transport
// error leaves execution uncertain; do not retry side effects under a new ID.
// Protocol errors are returned as *protocol.Error and preserve the connection.
func (c *Client) Call(ctx context.Context, id, method string, params any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if params == nil {
		raw = json.RawMessage(`{}`)
	}
	req := protocol.Request{Version: protocol.Version, ID: id, Method: method, Params: raw}
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err = protocol.DecodeRequest(encoded); err != nil {
		return nil, err
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, io.ErrClosedPipe
	}
	defer func() { <-c.gate }()
	select {
	case <-c.done:
		return nil, io.ErrClosedPipe
	default:
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { c.Close(); close(stopped) })
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	if err = c.writer.Write(req); err != nil {
		c.Close()
		return nil, callError(ctx, err)
	}
	frame, err := c.reader.ReadFrame()
	if err != nil {
		c.Close()
		return nil, callError(ctx, err)
	}
	var response protocol.Response
	if err = json.Unmarshal(frame, &response); err != nil {
		c.Close()
		return nil, err
	}
	if response.Version != protocol.Version || response.RequestID != id || (response.Error == nil) == (len(response.Result) == 0) {
		c.Close()
		return nil, errors.New("invalid RPC response identity or result")
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result, nil
}
func callError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Hello negotiates protocol version 1 and returns the server's capabilities.
func (c *Client) Hello(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Call(ctx, id, "hello", nil)
}

// Prompt starts a run. The response is its durable execution record; poll events
// and request.get to observe progress and its terminal result.
func (c *Client) Prompt(ctx context.Context, id, text string) (json.RawMessage, error) {
	return c.Call(ctx, id, "prompt", struct {
		Text string `json:"text"`
	}{text})
}

// Cancel requests cancellation of the active run without closing the connection.
func (c *Client) Cancel(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Call(ctx, id, "cancel", nil)
}
