//go:build darwin || linux

package main

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// NewFile registers an already nonblocking descriptor with Go's poller.
// Inherited os.Stdin/os.Stdout may otherwise use blocking syscalls that Close
// cannot interrupt. RPC exclusively owns these duplicate descriptors.
func rpcPollableFile(source *os.File) (*os.File, error) {
	fd, err := syscall.Dup(int(source.Fd()))
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(fd)
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	file := os.NewFile(uintptr(fd), source.Name())
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("invalid RPC descriptor")
	}
	return file, nil
}

func prepareStdioRPC() (io.ReadWriteCloser, error) {
	return prepareRPCFiles(os.Stdin, os.Stdout)
}

func prepareRPCFiles(stdin, stdout *os.File) (io.ReadWriteCloser, error) {
	input, err := rpcPollableFile(stdin)
	if err != nil {
		return nil, fmt.Errorf("prepare RPC input: %w", err)
	}
	output, err := rpcPollableFile(stdout)
	if err != nil {
		input.Close()
		return nil, fmt.Errorf("prepare RPC output: %w", err)
	}
	remote, stop, err := watchRPCPeer(input, output)
	if err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	return &stdioRPC{input: input, output: output, remoteClosed: remote, stopRemote: stop}, nil
}

// SyscallConn preserves Go's nonblocking descriptor registration; File.Fd
// would switch the descriptor back to blocking mode.
func rpcDescriptor(file *os.File) (int, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return 0, err
	}
	var fd int
	err = raw.Control(func(value uintptr) { fd = int(value) })
	return fd, err
}
