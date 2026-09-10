package main

import (
	"errors"
	"io"
	"sync"

	"github.com/sausheong/hand/internal/app"
	handrpc "github.com/sausheong/hand/internal/rpc"
)

type stdioRPC struct {
	input        io.ReadCloser
	output       io.WriteCloser
	remoteClosed <-chan error
	stopRemote   func()
	closeOnce    sync.Once
	closeErr     error
}

func (s *stdioRPC) Read(p []byte) (int, error)  { return s.input.Read(p) }
func (s *stdioRPC) Write(p []byte) (int, error) { return s.output.Write(p) }
func (s *stdioRPC) RemoteClosed() <-chan error  { return s.remoteClosed }
func (s *stdioRPC) CloseWrite() error           { return s.output.Close() }
func (s *stdioRPC) Close() error {
	s.closeOnce.Do(func() {
		if s.stopRemote != nil {
			s.stopRemote()
		}
		s.closeErr = errors.Join(s.input.Close(), s.output.Close())
	})
	return s.closeErr
}
func runRPC(transport *handrpc.PreparedTransport, controller *app.Controller, storeDir, workspace string) error {
	ledger, err := handrpc.OpenWorkspaceLedger(storeDir, workspace)
	if err != nil {
		return err
	}
	defer ledger.Close()
	dispatcher := handrpc.NewControllerDispatcher(controller, ledger)
	return handrpc.ServePrepared(transport, dispatcher, handrpc.TransportOptions{})
}
