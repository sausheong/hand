package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sausheong/harness/runtime"
)

// StartupAttempt owns construction until Finish transfers a successful runtime.
// Cancel is nonblocking; Finish always joins construction before returning.
// A caller must finish an attempt before replacing it with a retry.
type StartupAttempt struct {
	ctx       context.Context
	cancel    context.CancelFunc
	build     func(context.Context) (*runtime.Runtime, error)
	start     sync.Once
	finish    sync.Once
	done      chan struct{}
	rt        *runtime.Runtime
	err       error
	result    *runtime.Runtime
	finishErr error
}

func NewStartupAttempt(parent context.Context, timeout time.Duration, build func(context.Context) (*runtime.Runtime, error)) *StartupAttempt {
	ctx, cancel := context.WithTimeout(parent, timeout)
	return &StartupAttempt{ctx: ctx, cancel: cancel, build: build, done: make(chan struct{})}
}

func (a *StartupAttempt) Start() {
	a.start.Do(func() {
		go func() {
			defer close(a.done)
			if a.build == nil {
				a.err = errors.New("missing runtime builder")
				return
			}
			a.rt, a.err = a.build(a.ctx)
			if a.err == nil && a.rt == nil {
				a.err = errors.New("runtime builder returned no runtime")
			}
		}()
	})
}

func (a *StartupAttempt) Done() <-chan struct{} { return a.done }
func (a *StartupAttempt) Cancel()               { a.cancel() }

// Err joins construction. It is intended for a completed-attempt notification.
func (a *StartupAttempt) Err() error {
	a.Start()
	<-a.done
	return a.err
}

// Finish must be called once by the owner. A nonnil cause abandons the runtime;
// successful transfer requires construction to have completed without cancellation.
func (a *StartupAttempt) Finish(cause error) (*runtime.Runtime, error) {
	a.finish.Do(func() {
		// Preserve a cancellation already requested before releasing the timer.
		cancelErr := a.ctx.Err()
		select {
		case <-a.done:
		default:
			cancelErr = errors.Join(cancelErr, context.Canceled)
		}
		a.cancel()
		a.Start()
		<-a.done
		a.finishErr = errors.Join(cause, cancelErr, a.err)
		a.result = a.rt
		if a.finishErr != nil && a.rt != nil {
			a.finishErr = errors.Join(a.finishErr, a.rt.Close())
			a.result = nil
		}
	})
	return a.result, a.finishErr
}
