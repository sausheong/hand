package rpc

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/sausheong/hand/protocol"
)

type incomingFrame struct {
	data []byte
	err  error
}

var ErrStartupBacklog = errors.New("RPC startup request queue full; wait for hello response before sending more requests")

// PreparedTransport owns a bounded reader before application construction.
// Disconnect cancels startup through the supplied callback; ServePrepared
// transfers that callback to the dispatcher without starting another reader.
// Close on conn must unblock both Read and Write.
type PreparedTransport struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	cancelCause           context.CancelCauseFunc
	conn                  io.ReadWriteCloser
	incoming              chan incomingFrame
	readerDone, watchDone chan struct{}
	mu                    sync.Mutex
	disconnected          bool
	starting              bool
	startupFrames         int
	closeError            error
	startupError          error
	inputError            error
	onDisconnect          func()
}

func PrepareTransport(parent context.Context, conn io.ReadWriteCloser, onDisconnect func()) *PreparedTransport {
	return prepareTransport(parent, conn, onDisconnect, true)
}

func prepareTransport(parent context.Context, conn io.ReadWriteCloser, onDisconnect func(), starting bool) *PreparedTransport {
	ctx, cancelCause := context.WithCancelCause(parent)
	t := &PreparedTransport{ctx: ctx, cancel: func() { cancelCause(context.Canceled) }, cancelCause: cancelCause, conn: conn, incoming: make(chan incomingFrame, 1),
		readerDone: make(chan struct{}), watchDone: make(chan struct{}), onDisconnect: onDisconnect, starting: starting}
	var remote <-chan error
	// With a half-close-capable notifier, io.EOF guarantees input writers
	// have gone. Drain finite remaining input to retain framing errors.
	half, canHalfClose := conn.(interface{ CloseWrite() error })
	if notifier, ok := conn.(interface{ RemoteClosed() <-chan error }); ok {
		remote = notifier.RemoteClosed()
	}
	go func() {
		defer close(t.watchDone)
		select {
		case <-ctx.Done():
		case cause := <-remote:
			if cause == nil {
				cause = io.EOF
			}
			t.cancelCause(cause)
		}
		t.disconnect()
		if canHalfClose && errors.Is(context.Cause(ctx), io.EOF) {
			_ = half.CloseWrite()
			<-t.readerDone
		}
		t.closeError = conn.Close()
	}()
	go func() {
		defer close(t.readerDone)
		reader := protocol.NewReader(conn)
		for {
			data, err := reader.ReadFrame()
			if err == nil && t.recordStartupFrame() {
				return
			}
			if err != nil {
				t.mu.Lock()
				var framing *protocol.Error
				if errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &framing) {
					t.inputError = err
				}
				t.mu.Unlock()
				t.disconnect()
			}
			if ctx.Err() != nil && canHalfClose && errors.Is(context.Cause(ctx), io.EOF) {
				if err != nil {
					return
				}
				continue
			}
			select {
			case t.incoming <- incomingFrame{data, err}:
			case <-ctx.Done():
				if canHalfClose && errors.Is(context.Cause(ctx), io.EOF) && err == nil {
					continue
				}
				return
			default:
				// A second complete frame before handoff would block this
				// reader from seeing EOF while construction waits on MCP.
				// After handoff, preserve the normal bounded backpressure.
				if err == nil && t.rejectStartupBacklog() {
					return
				}
				select {
				case t.incoming <- incomingFrame{data, err}:
				case <-ctx.Done():
					if canHalfClose && errors.Is(context.Cause(ctx), io.EOF) && err == nil {
						continue
					}
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return t
}

func (t *PreparedTransport) cancellationError() error {
	if errors.Is(context.Cause(t.ctx), io.EOF) || errors.Is(context.Cause(t.ctx), io.ErrClosedPipe) {
		return nil
	}
	return context.Cause(t.ctx)
}

// Count complete startup frames even while draining a notified EOF. The
// cancellation path must not erase a backlog diagnostic from buffered input.
func (t *PreparedTransport) recordStartupFrame() bool {
	t.mu.Lock()
	if t.starting {
		t.startupFrames++
	}
	overflow := t.starting && t.startupFrames > 1
	t.mu.Unlock()
	return overflow && t.rejectStartupBacklog()
}

func (t *PreparedTransport) rejectStartupBacklog() bool {
	t.mu.Lock()
	starting := t.starting
	if starting {
		t.startupError = ErrStartupBacklog
	}
	t.mu.Unlock()
	if starting {
		t.disconnect()
		t.cancel()
	}
	return starting
}

func (t *PreparedTransport) StartupError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.startupError == nil {
		return t.inputError
	}
	return t.startupError
}

func (t *PreparedTransport) disconnect() {
	t.mu.Lock()
	t.disconnected = true
	callback := t.onDisconnect
	t.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func (t *PreparedTransport) transfer(callback func()) {
	t.mu.Lock()
	t.starting = false
	t.onDisconnect = callback
	disconnected := t.disconnected
	t.mu.Unlock()
	if disconnected && callback != nil {
		callback()
	}
}

func (t *PreparedTransport) Close() error {
	t.cancel()
	// The watcher owns connection closure. On notified EOF it first drains
	// finite buffered input; closing here would race away framing/backlog errors.
	<-t.watchDone
	<-t.readerDone
	return t.closeError
}
