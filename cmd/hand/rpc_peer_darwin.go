package main

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// EV_CLEAR observes changes without spinning on queued input or writable output.
// EV_EOF remains observable even when the application's input queue is full.
func watchRPCPeer(input, output *os.File) (<-chan error, func(), error) {
	inFD, err := rpcDescriptor(input)
	if err != nil {
		return nil, nil, err
	}
	outFD, err := rpcDescriptor(output)
	if err != nil {
		return nil, nil, err
	}
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, nil, err
	}
	unix.CloseOnExec(kq)
	wakeRead, wakeWrite, err := os.Pipe()
	if err != nil {
		unix.Close(kq)
		return nil, nil, err
	}
	wakeFD := int(wakeRead.Fd())
	changes := []unix.Kevent_t{
		{Ident: uint64(inFD), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD | unix.EV_CLEAR},
		{Ident: uint64(outFD), Filter: unix.EVFILT_WRITE, Flags: unix.EV_ADD | unix.EV_CLEAR},
		{Ident: uint64(wakeFD), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD | unix.EV_CLEAR},
	}
	if _, err := unix.Kevent(kq, changes, nil, nil); err != nil {
		wakeRead.Close()
		wakeWrite.Close()
		unix.Close(kq)
		return nil, nil, fmt.Errorf("watch RPC peer: %w", err)
	}
	remote, done := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(done)
		events := make([]unix.Kevent_t, 3)
		for {
			n, err := unix.Kevent(kq, nil, events, nil)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				remote <- err
				return
			}
			for _, event := range events[:n] {
				if event.Ident == uint64(wakeFD) {
					return
				}
				if event.Flags&unix.EV_ERROR != 0 {
					remote <- unix.Errno(event.Data)
					return
				}
				if event.Flags&unix.EV_EOF != 0 {
					if event.Ident == uint64(inFD) {
						remote <- io.EOF
					} else {
						remote <- io.ErrClosedPipe
					}
					return
				}
			}
		}
	}()
	var once sync.Once
	stop := func() { once.Do(func() { wakeWrite.Close(); <-done; wakeRead.Close(); unix.Close(kq) }) }
	return remote, stop, nil
}
