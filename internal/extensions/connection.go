// Package extensions owns extension connections. Creating a connection does not
// discover, launch or authorise code; its caller supplies an admitted transport.
package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/harness/process"
)

// Transport must make Stop terminate the whole owned process tree and unblock
// its pipes. Wait must join process/backend cleanup. Stop must be concurrency-safe.
type Transport struct {
	Input    io.WriteCloser
	Output   io.ReadCloser
	Stderr   io.ReadCloser
	Stop     func()
	Wait     func() error
	Boundary string
}
type ResponseError struct{ Code, Message string }

func (e *ResponseError) Error() string { return e.Code + ": " + e.Message }

type pendingCall struct {
	ctx       context.Context
	callbacks map[string]bool
	id        string
	reply     chan protocol.Frame
}
type Connection struct {
	transport          Transport
	writer             *protocol.Writer
	ctx                context.Context
	cancel             context.CancelCauseFunc
	once               sync.Once
	mu                 sync.Mutex
	pending            *pendingCall
	handler            CallbackHandler
	capabilities       map[string]bool
	handshakeAttempted bool
	gate               chan struct{}
	sequence           atomic.Uint64
	diagnostics        *process.Capture
	done               chan struct{}
	workers            sync.WaitGroup
	waitError          error
}

func Connect(ctx context.Context, t Transport) (*Connection, error) {
	if t.Input == nil || t.Output == nil || t.Stderr == nil || t.Stop == nil || t.Wait == nil || t.Boundary == "" {
		return nil, errors.New("incomplete admitted extension transport")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owned, cancel := context.WithCancelCause(ctx)
	c := &Connection{transport: t, writer: protocol.NewWriter(t.Input), ctx: owned, cancel: cancel, gate: make(chan struct{}, 1), diagnostics: process.NewCapture(64 << 10), done: make(chan struct{})}
	c.workers.Add(2)
	go c.read()
	go c.drainDiagnostics()
	go func() {
		err := t.Wait()
		c.waitError = err
		if err == nil {
			err = io.EOF
		}
		c.stop(fmt.Errorf("extension exited: %w", err))
		c.workers.Wait()
		close(c.done)
	}()
	go func() {
		select {
		case <-owned.Done():
			c.stop(context.Cause(owned))
		case <-c.done:
		}
	}()
	return c, nil
}
func (c *Connection) stop(err error) {
	c.once.Do(func() {
		c.cancel(err)
		c.transport.Stop()
		c.transport.Input.Close()
		c.transport.Output.Close()
		c.transport.Stderr.Close()
	})
}
func (c *Connection) read() {
	defer c.workers.Done()
	reader := protocol.NewReader(c.transport.Output)
	for {
		f, err := reader.Read()
		if err != nil {
			c.stop(fmt.Errorf("extension protocol: %w", err))
			return
		}
		if f.Kind == "request" {
			if err := c.callback(f); err != nil {
				c.stop(err)
				return
			}
			continue
		}
		c.mu.Lock()
		pending := c.pending
		if f.Kind != "response" || pending == nil || f.ID != pending.id {
			c.mu.Unlock()
			c.stop(errors.New("unexpected extension frame or response ID"))
			return
		}
		c.pending = nil
		pending.reply <- f
		c.mu.Unlock()
	}
}
func (c *Connection) drainDiagnostics() {
	defer c.workers.Done()
	// Capture stays bounded; terminate a peer after 1 MiB rather than allowing
	// unlimited diagnostic traffic to consume host CPU for the entire session.
	n, err := io.Copy(c.diagnostics, io.LimitReader(c.transport.Stderr, (1<<20)+1))
	if n > 1<<20 {
		c.stop(errors.New("extension stderr exceeded 1 MiB"))
	} else if err != nil && c.ctx.Err() == nil {
		c.stop(fmt.Errorf("extension stderr: %w", err))
	}
}

// Call serialises host requests. A timeout during an admitted request terminates
// the peer: continuing would leave unknown side effects and response ordering.
// Callbacks require a validated handshake and an explicitly installed handler.
func (c *Connection) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	bounded, cancel := context.WithTimeout(ctx, requestTimeout(method, params))
	defer cancel()
	select {
	case c.gate <- struct{}{}:
	case <-bounded.Done():
		return nil, context.Cause(bounded)
	case <-c.ctx.Done():
		return nil, context.Cause(c.ctx)
	}
	defer func() { <-c.gate }()
	if err := bounded.Err(); err != nil {
		return nil, err
	}
	if c.ctx.Err() != nil {
		return nil, context.Cause(c.ctx)
	}
	id := "host-" + strconv.FormatUint(c.sequence.Add(1), 10)
	f := protocol.Frame{Version: protocol.Version, Kind: "request", ID: id, Method: method, Params: append(json.RawMessage(nil), params...)}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	// Validate and size the whole frame before exposing a pending request.
	var encoded countingWriter
	if err := protocol.NewWriter(&encoded).Write(f); err != nil {
		return nil, err
	}
	pending := &pendingCall{id: id, reply: make(chan protocol.Frame, 1), ctx: bounded, callbacks: make(map[string]bool)}
	c.mu.Lock()
	c.pending = pending
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.pending == pending {
			c.pending = nil
		}
		c.mu.Unlock()
	}()
	sent := make(chan error, 1)
	go func() { sent <- c.writer.Write(f) }()
	select {
	case err := <-sent:
		if err != nil {
			c.stop(err)
			<-c.done
			return nil, err
		}
	case <-bounded.Done():
		c.stop(context.Cause(bounded))
		<-sent
		<-c.done
		return nil, context.Cause(bounded)
	case <-c.ctx.Done():
		c.stop(context.Cause(c.ctx))
		<-sent
		<-c.done
		return nil, context.Cause(c.ctx)
	}
	select {
	case reply := <-pending.reply:
		if reply.Error != nil {
			return nil, &ResponseError{Code: reply.Error.Code, Message: reply.Error.Message}
		}
		return reply.Result, nil
	case <-bounded.Done():
		c.stop(context.Cause(bounded))
		<-c.done
		return nil, context.Cause(bounded)
	case <-c.ctx.Done():
		<-c.done
		return nil, context.Cause(c.ctx)
	}
}

type countingWriter struct{}

func (*countingWriter) Write(p []byte) (int, error) { return len(p), nil }
func (c *Connection) Close() error {
	c.stop(context.Canceled)
	<-c.done
	cause := context.Cause(c.ctx)
	if errors.Is(cause, context.Canceled) {
		if errors.Is(c.waitError, ErrTransportCleanup) {
			return c.waitError
		}
		return nil
	}
	return cause
}
func (c *Connection) Status() (boundary, diagnostics string, bytes int64, truncated bool, err error) {
	diagnostics, bytes, truncated = c.diagnostics.Snapshot()
	return c.transport.Boundary, diagnostics, bytes, truncated, context.Cause(c.ctx)
}
