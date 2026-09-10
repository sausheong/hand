package rpc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/protocol"
)

type bufferedStartupEOF struct {
	reader                      *bytes.Reader
	remote                      chan error
	release, halfClosed, closed chan struct{}
	halfOnce, closeOnce         sync.Once
}

func (c *bufferedStartupEOF) Read(p []byte) (int, error) {
	select {
	case <-c.release:
	case <-c.closed:
		return 0, io.ErrClosedPipe
	}
	return c.reader.Read(p)
}
func (c *bufferedStartupEOF) Write(p []byte) (int, error) { return len(p), nil }
func (c *bufferedStartupEOF) RemoteClosed() <-chan error  { return c.remote }
func (c *bufferedStartupEOF) CloseWrite() error {
	c.halfOnce.Do(func() { close(c.halfClosed) })
	return nil
}
func (c *bufferedStartupEOF) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestPreparedTransportCloseDrainsStartupBacklogAfterNotifiedEOF(t *testing.T) {
	var data bytes.Buffer
	for _, id := range []string{"first", "second"} {
		if err := protocol.NewWriter(&data).Write(rpcRequest(id, "hello", `{}`)); err != nil {
			t.Fatal(err)
		}
	}
	conn := &bufferedStartupEOF{reader: bytes.NewReader(data.Bytes()), remote: make(chan error, 1), release: make(chan struct{}), halfClosed: make(chan struct{}), closed: make(chan struct{})}
	conn.remote <- io.EOF
	transport := PrepareTransport(context.Background(), conn, func() {})
	defer func() {
		select {
		case <-conn.release:
		default:
			close(conn.release)
		}
		_ = transport.Close()
	}()
	// Hold the reader until the independent EOF notification has won the race.
	select {
	case <-conn.halfClosed:
	case <-time.After(time.Second):
		t.Fatal("EOF notifier was not processed")
	}
	closed := make(chan error, 1)
	go func() { closed <- transport.Close() }()
	// Close must retain the finite buffered input until the reader drains it.
	select {
	case <-conn.closed:
		close(conn.release)
		<-closed
		t.Fatal("connection closed before buffered startup input drained")
	case <-time.After(50 * time.Millisecond):
	}
	close(conn.release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("draining Close did not join")
	}
	if !errors.Is(transport.StartupError(), ErrStartupBacklog) {
		t.Fatalf("lost startup backlog: %v", transport.StartupError())
	}
}
