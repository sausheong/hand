//go:build linux

package main

import (
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"sync"
	"syscall"
)

// Observe pipe hangup without consuming input. A full decoded-frame queue must
// not hide peer closure from a blocked application operation. The wake pipe
// makes shutdown immediate without periodic polling or an abandoned goroutine.
func watchRPCPeer(input, output *os.File) (<-chan error, func(), error) {
	wakeRead, wakeWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	remote := make(chan error, 1)
	done := make(chan struct{})
	inFD, err := rpcDescriptor(input)
	if err != nil {
		wakeRead.Close()
		wakeWrite.Close()
		return nil, nil, err
	}
	outFD, err := rpcDescriptor(output)
	if err != nil {
		wakeRead.Close()
		wakeWrite.Close()
		return nil, nil, err
	}
	fds := []unix.PollFd{{Fd: int32(inFD)}, {Fd: int32(outFD)}, {Fd: int32(wakeRead.Fd()), Events: unix.POLLIN}}
	go func() {
		defer close(done)
		for {
			_, err := unix.Poll(fds, -1)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				remote <- fmt.Errorf("watch RPC peer: %w", err)
				return
			}
			if fds[2].Revents != 0 {
				return
			}
			if fds[0].Revents&unix.POLLHUP != 0 {
				remote <- io.EOF
				return
			}
			if fds[1].Revents&(unix.POLLHUP|unix.POLLERR) != 0 {
				remote <- io.ErrClosedPipe
				return
			}
			if (fds[0].Revents|fds[1].Revents)&(unix.POLLERR|unix.POLLNVAL) != 0 {
				remote <- fmt.Errorf("RPC pipe poll failure")
				return
			}
		}
	}()
	var once sync.Once
	stop := func() { once.Do(func() { wakeWrite.Close(); <-done; wakeRead.Close() }) }
	return remote, stop, nil
}
