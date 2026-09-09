package rpc

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/sausheong/hand/protocol"
)

type TransportOptions struct{ WriteTimeout time.Duration }

var ErrWriteTimeout = errors.New("RPC client did not read response before write deadline")

// Serve owns conn and dispatcher. Close on conn must unblock Read and Write.
// It reads at most one queued request ahead and joins its reader and active
// application work before returning. The caller continues to own the ledger.
func Serve(parent context.Context, conn io.ReadWriteCloser, dispatcher *Dispatcher, options TransportOptions) (result error) {
	return ServePrepared(prepareTransport(parent, conn, dispatcher.cancel, false), dispatcher, options)
}

// ServePrepared consumes the reader started before application construction.
// It owns the prepared transport and dispatcher; the caller owns the ledger.
func ServePrepared(transport *PreparedTransport, dispatcher *Dispatcher, options TransportOptions) (result error) {
	timeout := options.WriteTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport.transfer(dispatcher.cancel)
	ctx, conn := transport.ctx, transport.conn
	defer func() {
		// The peer monitor can observe EOF before trailing-frame validation.
		if errors.Is(context.Cause(ctx), io.EOF) {
			<-transport.readerDone
		}
		_ = transport.Close()
		dispatcher.Close()
		if result == nil {
			transport.mu.Lock()
			result = transport.inputError
			transport.mu.Unlock()
		}
	}()
	writer := protocol.NewWriter(conn)
	for {
		select {
		case <-ctx.Done():
			return transport.cancellationError()
		case f := <-transport.incoming:
			if ctx.Err() != nil {
				return transport.cancellationError()
			}
			if f.err != nil {
				if errors.Is(f.err, io.EOF) {
					return nil
				}
				return f.err
			}
			request, err := protocol.DecodeRequest(f.data)
			var response protocol.Response
			if err != nil {
				var detail *protocol.Error
				if !errors.As(err, &detail) {
					detail = &protocol.Error{Code: "invalid_request", Message: "invalid JSON request"}
				}
				response = protocol.Response{Version: protocol.Version, Error: detail}
			} else {
				response = dispatcher.Dispatch(ctx, request)
			}
			if ctx.Err() != nil {
				return transport.cancellationError()
			}
			timedOut := make(chan struct{})
			timer := time.AfterFunc(timeout, func() { _ = conn.Close(); close(timedOut) })
			err = writer.Write(response)
			if !timer.Stop() {
				<-timedOut
				return ErrWriteTimeout
			}
			if err != nil {
				if ctx.Err() != nil {
					return transport.cancellationError()
				}
				return err
			}
		}
	}
}
